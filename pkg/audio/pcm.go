package audio

func encodePCM16LE(dst []byte, samples []int16) []byte {
	size := len(samples) * 2
	if cap(dst) < size {
		dst = make([]byte, size)
	} else {
		dst = dst[:size]
	}
	for i, sample := range samples {
		dst[i*2] = byte(sample)
		dst[i*2+1] = byte(sample >> 8)
	}
	return dst
}
