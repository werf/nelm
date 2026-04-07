package util

import "fmt"

func Uint64ToInt(v uint64) int {
	maxInt := uint64(int(^uint(0) >> 1))
	if v > maxInt {
		panic(fmt.Sprintf("uint64 value %d overflows int", v))
	}

	return int(v)
}
