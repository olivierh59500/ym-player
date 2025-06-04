package stsound

import (
	"encoding/binary"
	"unsafe"
)

// Détection de l'endianness du système
var NativeEndian binary.ByteOrder

func init() {
	// Détecter l'endianness native
	buf := [2]byte{}
	*(*uint16)(unsafe.Pointer(&buf[0])) = uint16(0xABCD)

	if buf[0] == 0xCD && buf[1] == 0xAB {
		NativeEndian = binary.LittleEndian
	} else {
		NativeEndian = binary.BigEndian
	}
}

// IsLittleEndian retourne true si le système est little-endian
func IsLittleEndian() bool {
	return NativeEndian == binary.LittleEndian
}

// IsBigEndian retourne true si le système est big-endian
func IsBigEndian() bool {
	return NativeEndian == binary.BigEndian
}

// SwapBytes16 inverse les octets d'un uint16
func SwapBytes16(v uint16) uint16 {
	return (v << 8) | (v >> 8)
}

// SwapBytes32 inverse les octets d'un uint32
func SwapBytes32(v uint32) uint32 {
	return ((v & 0x000000FF) << 24) |
		((v & 0x0000FF00) << 8) |
		((v & 0x00FF0000) >> 8) |
		((v & 0xFF000000) >> 24)
}

// ConvertToNative convertit depuis big-endian vers l'ordre natif
func ConvertToNative32(v uint32) uint32 {
	if IsLittleEndian() {
		return SwapBytes32(v)
	}
	return v
}

// ConvertToNative16 convertit depuis big-endian vers l'ordre natif
func ConvertToNative16(v uint16) uint16 {
	if IsLittleEndian() {
		return SwapBytes16(v)
	}
	return v
}