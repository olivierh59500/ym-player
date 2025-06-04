package lzh

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// LZH constants from original C++ code
const (
	CHAR_BIT  = 8
	UCHAR_MAX = 255
	BITBUFSIZ = 16
	DICBIT    = 13
	DICSIZ    = 1 << DICBIT
	MAXMATCH  = 256
	THRESHOLD = 3
	NC        = UCHAR_MAX + MAXMATCH + 2 - THRESHOLD
	CBIT      = 9
	CODE_BIT  = 16
	NP        = DICBIT + 1
	NT        = CODE_BIT + 3
	PBIT      = 4
	TBIT      = 5
	NPT       = NT // NT > NP
	BUFSIZE   = 4096
)

// Decoder structure
type Decoder struct {
	// Input/Output
	input  *bytes.Reader
	output *bytes.Buffer

	// Bit buffer
	bitbuf      uint16
	subbitbuf   uint8
	bitcount    int
	fillbufsize int
	fillbuf_i   int
	buf         [BUFSIZE]byte

	// Huffman trees
	left     [2*NC - 1]uint16
	right    [2*NC - 1]uint16
	c_len    [NC]uint8
	pt_len   [NPT]uint8
	c_table  [4096]uint16
	pt_table [256]uint16

	// Decode state
	blocksize uint16
	decode_j  int
	decode_i  uint32
	outbuf    [DICSIZ]uint8
}

// Decompress decompresses LH5 compressed data
func Decompress(data []byte) ([]byte, error) {
	if len(data) < 7 {
		return nil, errors.New("data too small")
	}

	// Find LZH header by looking for -lhX- pattern
	headerStart := -1
	for i := 0; i <= len(data)-7; i++ {
		if data[i+2] == '-' && data[i+3] == 'l' && data[i+4] == 'h' && data[i+6] == '-' {
			headerStart = i
			break
		}
	}

	if headerStart < 0 {
		return nil, errors.New("LZH header not found")
	}

	reader := bytes.NewReader(data[headerStart:])

	// Read header
	var header struct {
		HeaderSize   uint8
		HeaderSum    uint8
		Method       [5]uint8
		PackedSize   uint32
		OriginalSize uint32
		FileTime     uint32
		Attribute    uint8
		Level        uint8
	}

	// Read header size
	if err := binary.Read(reader, binary.LittleEndian, &header.HeaderSize); err != nil {
		return nil, err
	}

	// Read header checksum
	if err := binary.Read(reader, binary.LittleEndian, &header.HeaderSum); err != nil {
		return nil, err
	}

	// Read method
	if _, err := reader.Read(header.Method[:]); err != nil {
		return nil, err
	}

	methodStr := string(header.Method[:])
	if methodStr != "-lh5-" && methodStr != "-lh4-" && methodStr != "-lh0-" {
		return nil, fmt.Errorf("unsupported method: %s", methodStr)
	}

	// Read sizes
	if err := binary.Read(reader, binary.LittleEndian, &header.PackedSize); err != nil {
		return nil, err
	}
	if err := binary.Read(reader, binary.LittleEndian, &header.OriginalSize); err != nil {
		return nil, err
	}

	// Skip the rest of the header
	// We've read: 1 + 1 + 5 + 4 + 4 = 15 bytes
	// Total header size is HeaderSize + 2
	toSkip := int(header.HeaderSize) + 2 - 15
	if toSkip > 0 {
		if _, err := reader.Seek(int64(toSkip), 1); err != nil {
			return nil, err
		}
	}

	// For -lh0-, data is uncompressed
	if methodStr == "-lh0-" {
		output := make([]byte, header.OriginalSize)
		n, err := reader.Read(output)
		if err != nil && err != io.EOF {
			return nil, err
		}
		if n != int(header.OriginalSize) {
			return nil, fmt.Errorf("incomplete data: got %d, expected %d", n, header.OriginalSize)
		}
		return output, nil
	}

	// Read compressed data
	compressedData := make([]byte, header.PackedSize)
	n, err := reader.Read(compressedData)
	if err != nil && err != io.EOF {
		return nil, err
	}

	// Create decoder
	decoder := &Decoder{
		input:  bytes.NewReader(compressedData[:n]),
		output: bytes.NewBuffer(make([]byte, 0, header.OriginalSize)),
	}

	// Decode
	if err := decoder.decode(int(header.OriginalSize)); err != nil {
		return nil, err
	}

	return decoder.output.Bytes(), nil
}

func (d *Decoder) fillbuf(n int) {
	d.bitbuf = (d.bitbuf << n) & 0xffff
	for n > d.bitcount {
		d.bitbuf |= uint16(d.subbitbuf) << (n - d.bitcount)
		n -= d.bitcount

		if d.fillbufsize == 0 {
			d.fillbuf_i = 0
			nread, _ := d.input.Read(d.buf[:BUFSIZE-32])
			d.fillbufsize = nread
		}

		if d.fillbufsize > 0 {
			d.fillbufsize--
			d.subbitbuf = d.buf[d.fillbuf_i]
			d.fillbuf_i++
		} else {
			d.subbitbuf = 0
		}
		d.bitcount = CHAR_BIT
	}
	d.bitcount -= n
	d.bitbuf |= uint16(d.subbitbuf) >> d.bitcount
}

func (d *Decoder) getbits(n int) uint16 {
	x := d.bitbuf >> (BITBUFSIZ - n)
	d.fillbuf(n)
	return x
}

func (d *Decoder) init_getbits() {
	d.bitbuf = 0
	d.subbitbuf = 0
	d.bitcount = 0
	d.fillbufsize = 0
	d.fillbuf(BITBUFSIZ)
}

func (d *Decoder) make_table(nchar int, bitlen []uint8, tablebits int, table []uint16) {
	var count [17]uint16
	var weight [17]uint16
	var start [18]uint16

	// Count bit lengths
	for i := 1; i <= 16; i++ {
		count[i] = 0
	}
	for i := 0; i < nchar; i++ {
		if bitlen[i] > 0 && bitlen[i] <= 16 {
			count[bitlen[i]]++
		}
	}

	// Calculate starting code values
	start[1] = 0
	for i := 1; i <= 16; i++ {
		start[i+1] = start[i] + (count[i] << (16 - i))
	}
	// Check for valid table - in the C++ code, this returns 1 for error
	// but we'll just continue as the C++ code doesn't check the return value

	// Assign weights
	jutbits := 16 - tablebits
	for i := 1; i <= tablebits; i++ {
		start[i] >>= jutbits
		weight[i] = 1 << (tablebits - i)
	}
	for i := tablebits + 1; i <= 16; i++ {
		weight[i] = 1 << (16 - i)
	}

	// Initialize table
	i := int(start[tablebits+1] >> jutbits)
	if i != 0 && i < (1<<16) {
		k := 1 << tablebits
		for j := i; j < k && j < len(table); j++ {
			table[j] = 0
		}
	}

	// Make table
	avail := uint16(nchar)
	mask := uint16(1 << (15 - tablebits))

	for ch := 0; ch < nchar; ch++ {
		bitLength := int(bitlen[ch])
		if bitLength == 0 {
			continue
		}

		nextcode := start[bitLength] + weight[bitLength]
		if bitLength <= tablebits {
			for i := int(start[bitLength]); i < int(nextcode) && i < len(table); i++ {
				table[i] = uint16(ch)
			}
		} else {
			k := start[bitLength]
			idx := int(k >> jutbits)
			if idx >= len(table) {
				continue
			}
			p := &table[idx]
			remaining := bitLength - tablebits
			for remaining > 0 {
				if *p == 0 {
					if int(avail) >= len(d.left) {
						break
					}
					d.right[avail] = 0
					d.left[avail] = 0
					*p = avail
					avail++
				}
				if int(*p) >= len(d.left) {
					break
				}
				if (k & mask) != 0 {
					p = &d.right[*p]
				} else {
					p = &d.left[*p]
				}
				k <<= 1
				remaining--
			}
			if remaining == 0 {
				*p = uint16(ch)
			}
		}
		start[bitLength] = nextcode
	}
}

func (d *Decoder) read_pt_len(nn, nbit, i_special int) {
	n := d.getbits(nbit)

	if n == 0 {
		c := d.getbits(nbit)
		for i := 0; i < nn; i++ {
			d.pt_len[i] = 0
		}
		for i := 0; i < 256; i++ {
			d.pt_table[i] = c
		}
	} else {
		i := 0
		for i < int(n) {
			c := int(d.bitbuf >> (BITBUFSIZ - 3))
			if c == 7 {
				mask := uint16(1 << (BITBUFSIZ - 1 - 3))
				for (mask & d.bitbuf) != 0 {
					mask >>= 1
					c++
				}
			}
			var fillLen int
			if c < 7 {
				fillLen = 3
			} else {
				fillLen = c - 3
			}
			d.fillbuf(fillLen)
			d.pt_len[i] = uint8(c)
			i++

			if i == i_special {
				c := d.getbits(2)
				for c > 0 {
					d.pt_len[i] = 0
					i++
					c--
				}
			}
		}
		for i < nn {
			d.pt_len[i] = 0
			i++
		}
		d.make_table(nn, d.pt_len[:], 8, d.pt_table[:])
	}
}

func (d *Decoder) read_c_len() {
	n := d.getbits(CBIT)

	if n == 0 {
		c := d.getbits(CBIT)
		for i := 0; i < NC; i++ {
			d.c_len[i] = 0
		}
		for i := 0; i < 4096; i++ {
			d.c_table[i] = c
		}
	} else {
		i := 0
		for i < int(n) {
			c := d.pt_table[d.bitbuf>>(BITBUFSIZ-8)]
			if c >= NT {
				mask := uint16(1 << (BITBUFSIZ - 1 - 8))
				for c >= NT {
					if (d.bitbuf & mask) != 0 {
						c = d.right[c]
					} else {
						c = d.left[c]
					}
					mask >>= 1
				}
			}
			d.fillbuf(int(d.pt_len[c]))

			if c <= 2 {
				if c == 0 {
					c = 1
				} else if c == 1 {
					c = d.getbits(4) + 3
				} else {
					c = d.getbits(CBIT) + 20
				}
				for c > 0 {
					d.c_len[i] = 0
					i++
					c--
				}
			} else {
				d.c_len[i] = uint8(c - 2)
				i++
			}
		}
		for i < NC {
			d.c_len[i] = 0
			i++
		}
		d.make_table(NC, d.c_len[:], 12, d.c_table[:])
	}
}

func (d *Decoder) decode_c() uint16 {
	if d.blocksize == 0 {
		d.blocksize = d.getbits(16)
		d.read_pt_len(NT, TBIT, 3)
		d.read_c_len()
		d.read_pt_len(NP, PBIT, -1)
	}
	d.blocksize--

	j := d.c_table[d.bitbuf>>(BITBUFSIZ-12)]
	if j >= NC {
		mask := uint16(1 << (BITBUFSIZ - 1 - 12))
		for j >= NC {
			if (d.bitbuf & mask) != 0 {
				j = d.right[j]
			} else {
				j = d.left[j]
			}
			mask >>= 1
		}
	}
	d.fillbuf(int(d.c_len[j]))
	return j
}

func (d *Decoder) decode_p() uint16 {
	j := d.pt_table[d.bitbuf>>(BITBUFSIZ-8)]
	if j >= NP {
		mask := uint16(1 << (BITBUFSIZ - 1 - 8))
		for j >= NP {
			if (d.bitbuf & mask) != 0 {
				j = d.right[j]
			} else {
				j = d.left[j]
			}
			mask >>= 1
		}
	}
	d.fillbuf(int(d.pt_len[j]))
	if j != 0 {
		j--
		j = (1 << j) + d.getbits(int(j))
	}
	return j
}

func (d *Decoder) decode(origSize int) error {
	// Initialize
	d.init_getbits()
	d.blocksize = 0
	d.decode_j = 0

	for origSize > 0 {
		count := origSize
		if count > DICSIZ {
			count = DICSIZ
		}

		// Decode into buffer
		d.decodeBuffer(count)

		// Write to output
		if _, err := d.output.Write(d.outbuf[:count]); err != nil {
			return err
		}

		origSize -= count
	}

	return nil
}

func (d *Decoder) decodeBuffer(count int) {
	r := uint32(0)

	for d.decode_j > 0 && r < uint32(count) {
		d.outbuf[r] = d.outbuf[d.decode_i]
		d.decode_i = (d.decode_i + 1) & (DICSIZ - 1)
		r++
		d.decode_j--
	}

	for r < uint32(count) {
		c := d.decode_c()

		if c <= UCHAR_MAX {
			d.outbuf[r] = uint8(c)
			r++
		} else {
			d.decode_j = int(c) - (UCHAR_MAX + 1 - THRESHOLD)
			p := d.decode_p()
			d.decode_i = (r - uint32(p) - 1) & (DICSIZ - 1)

			for d.decode_j > 0 && r < uint32(count) {
				d.outbuf[r] = d.outbuf[d.decode_i]
				d.decode_i = (d.decode_i + 1) & (DICSIZ - 1)
				r++
				d.decode_j--
			}
		}
	}
}

// IsLZHCompressed checks if data is LZH compressed
func IsLZHCompressed(data []byte) bool {
	if len(data) < 7 {
		return false
	}
	// Check for -lhX- pattern at position 2
	return data[2] == '-' && data[3] == 'l' && data[4] == 'h' && data[6] == '-'
}

// GetCompressionMethod returns the compression method or empty string if not LZH
func GetCompressionMethod(data []byte) string {
	if !IsLZHCompressed(data) {
		return ""
	}
	return string(data[2:7])
}
