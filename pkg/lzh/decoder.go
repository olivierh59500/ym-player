package lzh

import (
	"encoding/binary"
	"errors"
	"fmt"
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
	input    []byte
	inputPos int

	// Bit buffer
	bitbuf        uint16
	subbitbuf     uint8
	bitcount      int
	bitsRemaining int

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
	decode_i  int
	outbuf    [DICSIZ]uint8
}

// Decompress decompresses LH0, LH4, or LH5 data.
func Decompress(data []byte) (output []byte, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			output = nil
			err = fmt.Errorf("invalid LZH data: %v", recovered)
		}
	}()

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

	if len(data)-headerStart < 24 {
		return nil, errors.New("truncated LZH header")
	}
	header := data[headerStart:]
	headerSize := int(header[0])
	if headerSize < 22 {
		return nil, fmt.Errorf("invalid LZH header size: %d", headerSize)
	}

	if header[20] != 0 {
		return nil, fmt.Errorf("unsupported LZH header level: %d", header[20])
	}
	nameEnd := 22 + int(header[21])
	if nameEnd+2 > len(header) {
		return nil, errors.New("truncated LZH filename or CRC")
	}

	method := header[5]
	if method != '5' && method != '4' && method != '0' {
		return nil, fmt.Errorf("unsupported method: %s", header[2:7])
	}

	packedSize := binary.LittleEndian.Uint32(header[7:11])
	originalSize := binary.LittleEndian.Uint32(header[11:15])
	payloadStart := headerStart + headerSize + 2
	if method == '5' {
		// ST-Sound derives the level-0 payload from the filename and CRC. Some
		// authentic archives contain a damaged size byte (notably 0x78), while
		// their filename length and compressed stream remain intact.
		payloadStart = headerStart + nameEnd + 2
	} else if headerSize+2 < nameEnd+2 {
		return nil, errors.New("LZH header does not contain its filename and CRC")
	}
	if payloadStart > len(data) {
		return nil, errors.New("truncated LZH header")
	}
	available := len(data) - payloadStart
	// Some historical YM archives have an inaccurate packed-size field but a
	// valid bitstream. Clamp to the available bytes as the original decoder did.
	payloadSize := int(min(uint64(packedSize), uint64(available)))
	payload := data[payloadStart : payloadStart+payloadSize]
	if uint64(originalSize) > uint64(^uint(0)>>1) {
		return nil, errors.New("uncompressed LZH size is too large")
	}

	// For -lh0-, data is uncompressed
	if method == '0' {
		if int(originalSize) > len(payload) {
			return nil, fmt.Errorf("incomplete data: got %d, expected %d", len(payload), originalSize)
		}
		output := make([]byte, int(originalSize))
		copy(output, payload)
		return output, nil
	}

	decoder := Decoder{input: payload}
	return decoder.decodeSize(int(originalSize)), nil
}

func (d *Decoder) fillbuf(n int) {
	if n < 0 || n > BITBUFSIZ || n > d.bitsRemaining {
		panic("truncated or invalid LZH bitstream")
	}
	d.bitsRemaining -= n
	d.bitbuf = (d.bitbuf << n) & 0xffff
	for n > d.bitcount {
		d.bitbuf |= uint16(d.subbitbuf) << (n - d.bitcount)
		n -= d.bitcount

		if d.inputPos < len(d.input) {
			d.subbitbuf = d.input[d.inputPos]
			d.inputPos++
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
	d.inputPos = 0
	// fillbuf keeps sixteen lookahead bits. These may be padded at EOF, but
	// the decoder may only consume bits actually present in the payload.
	d.bitsRemaining = len(d.input)*8 + BITBUFSIZ
	d.fillbuf(BITBUFSIZ)
}

func (d *Decoder) make_table(nchar int, bitlen []uint8, tablebits int, table []uint16) {
	var count [17]int
	var next [17]int
	for _, length := range bitlen[:nchar] {
		if length > 16 {
			panic("invalid LZH Huffman code length")
		}
		if length != 0 {
			count[length]++
		}
	}
	code := 0
	for length := 1; length <= 16; length++ {
		code = (code + count[length-1]) << 1
		next[length] = code
		if code+count[length] > 1<<length {
			panic("oversubscribed LZH Huffman tree")
		}
	}
	if code+count[16] != 1<<16 {
		panic("incomplete LZH Huffman tree")
	}
	clear(table)
	available := nchar
	for symbol, lengthByte := range bitlen[:nchar] {
		length := int(lengthByte)
		if length == 0 {
			continue
		}
		code := next[length]
		next[length]++
		if length <= tablebits {
			start := code << (tablebits - length)
			end := (code + 1) << (tablebits - length)
			for i := start; i < end; i++ {
				table[i] = uint16(symbol)
			}
			continue
		}
		entry := &table[code>>(length-tablebits)]
		for bit := length - tablebits - 1; bit >= 0; bit-- {
			if *entry == 0 {
				if available >= len(d.left) {
					panic("LZH Huffman tree is too large")
				}
				d.left[available], d.right[available] = 0, 0
				*entry = uint16(available)
				available++
			}
			if int(*entry) < nchar || int(*entry) >= available {
				panic("invalid LZH Huffman branch")
			}
			if code&(1<<bit) == 0 {
				entry = &d.left[*entry]
			} else {
				entry = &d.right[*entry]
			}
		}
		*entry = uint16(symbol)
	}
}

// treeSymbol resolves only the bounded number of branches beyond the lookup
// table, preventing a malformed archive from creating a cyclic tree walk.
func (d *Decoder) treeSymbol(symbol uint16, alphabet, tablebits int) uint16 {
	mask := uint16(1 << (BITBUFSIZ - 1 - tablebits))
	for symbol >= uint16(alphabet) {
		if mask == 0 || int(symbol) >= len(d.left) {
			panic("invalid LZH Huffman tree")
		}
		if d.bitbuf&mask == 0 {
			symbol = d.left[symbol]
		} else {
			symbol = d.right[symbol]
		}
		mask >>= 1
	}
	return symbol
}

func (d *Decoder) read_pt_len(nn, nbit, i_special int) {
	n := d.getbits(nbit)
	if int(n) > nn {
		panic("invalid LZH position table length")
	}

	if n == 0 {
		c := d.getbits(nbit)
		if int(c) >= nn {
			panic("invalid LZH position symbol")
		}
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
			if c > 16 {
				panic("invalid LZH position code length")
			}
			d.fillbuf(fillLen)
			d.pt_len[i] = uint8(c)
			i++

			if i == i_special {
				c := d.getbits(2)
				if i+int(c) > int(n) {
					panic("invalid LZH position zero run")
				}
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
	if int(n) > NC {
		panic("invalid LZH character table length")
	}

	if n == 0 {
		c := d.getbits(CBIT)
		if c >= NC {
			panic("invalid LZH character symbol")
		}
		for i := 0; i < NC; i++ {
			d.c_len[i] = 0
		}
		for i := 0; i < 4096; i++ {
			d.c_table[i] = c
		}
	} else {
		i := 0
		for i < int(n) {
			c := d.treeSymbol(d.pt_table[d.bitbuf>>(BITBUFSIZ-8)], NT, 8)
			d.fillbuf(int(d.pt_len[c]))

			if c <= 2 {
				if c == 0 {
					c = 1
				} else if c == 1 {
					c = d.getbits(4) + 3
				} else {
					c = d.getbits(CBIT) + 20
				}
				if i+int(c) > int(n) {
					panic("invalid LZH character zero run")
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
		if d.blocksize == 0 {
			panic("empty LZH Huffman block")
		}
		d.read_pt_len(NT, TBIT, 3)
		d.read_c_len()
		d.read_pt_len(NP, PBIT, -1)
	}
	d.blocksize--

	j := d.treeSymbol(d.c_table[d.bitbuf>>(BITBUFSIZ-12)], NC, 12)
	d.fillbuf(int(d.c_len[j]))
	return j
}

func (d *Decoder) decode_p() uint16 {
	j := d.treeSymbol(d.pt_table[d.bitbuf>>(BITBUFSIZ-8)], NP, 8)
	d.fillbuf(int(d.pt_len[j]))
	if j != 0 {
		j--
		j = (1 << j) + d.getbits(int(j))
	}
	return j
}

func (d *Decoder) decodeSize(size int) []byte {
	d.init_getbits()
	d.blocksize = 0
	d.decode_j = 0
	// Grow after decoding each block, so a truncated stream with a forged
	// original-size field does not trigger an immediate huge allocation.
	output := make([]byte, 0, min(size, DICSIZ))
	for len(output) < size {
		count := min(size-len(output), DICSIZ)
		d.decodeBuffer(count)
		output = append(output, d.outbuf[:count]...)
	}
	return output
}

func (d *Decoder) decodeBuffer(count int) {
	r := 0

	for d.decode_j > 0 && r < count {
		d.outbuf[r] = d.outbuf[d.decode_i]
		d.decode_i = (d.decode_i + 1) & (DICSIZ - 1)
		r++
		d.decode_j--
	}

	for r < count {
		c := d.decode_c()

		if c <= UCHAR_MAX {
			d.outbuf[r] = uint8(c)
			r++
		} else {
			d.decode_j = int(c) - (UCHAR_MAX + 1 - THRESHOLD)
			p := d.decode_p()
			distance := int(p) + 1
			d.decode_i = (r - distance) & (DICSIZ - 1)

			// Non-overlapping matches that do not wrap around the dictionary can
			// use the runtime's optimized copy implementation.
			if d.decode_j <= count-r && d.decode_j <= distance && r >= distance {
				copy(d.outbuf[r:r+d.decode_j], d.outbuf[d.decode_i:d.decode_i+d.decode_j])
				r += d.decode_j
				d.decode_i = (d.decode_i + d.decode_j) & (DICSIZ - 1)
				d.decode_j = 0
				continue
			}

			for d.decode_j > 0 && r < count {
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
