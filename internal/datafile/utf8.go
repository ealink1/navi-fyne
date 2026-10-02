package datafile

import (
	"errors"
	"unicode/utf8"

	"golang.org/x/text/transform"
)

type utf8Validator struct{}

func (*utf8Validator) Reset() {}
func (*utf8Validator) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for nSrc < len(src) {
		if !utf8.FullRune(src[nSrc:]) && !atEOF {
			return nDst, nSrc, transform.ErrShortSrc
		}
		r, size := utf8.DecodeRune(src[nSrc:])
		if r == utf8.RuneError && size == 1 {
			return nDst, nSrc, errors.New("file is not valid UTF-8; choose the correct encoding")
		}
		if len(dst)-nDst < size {
			return nDst, nSrc, transform.ErrShortDst
		}
		copy(dst[nDst:], src[nSrc:nSrc+size])
		nSrc += size
		nDst += size
	}
	return nDst, nSrc, nil
}
