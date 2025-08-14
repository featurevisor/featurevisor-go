package sdk

import "fmt"

// MurmurHashV3 implements the MurmurHash v3 algorithm
// Ported from the TypeScript implementation in the Featurevisor SDK
//
// Copyright (c) 2020 Gary Court, Derek Perez
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

// MurmurHashV3 calculates the MurmurHash v3 hash of the given key with the specified seed
func MurmurHashV3(key interface{}, seed uint32) uint32 {
	var data []byte

	// Convert key to byte slice
	switch v := key.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		// For other types, convert to string first
		data = []byte(fmt.Sprintf("%v", v))
	}

	length := len(data)
	remainder := length & 3 // length % 4
	bytes := length - remainder
	h1 := seed
	c1 := uint32(0xcc9e2d51)
	c2 := uint32(0x1b873593)
	i := 0

	for i < bytes {
		// Read 4 bytes as little-endian uint32
		k1 := uint32(data[i]) |
			(uint32(data[i+1]) << 8) |
			(uint32(data[i+2]) << 16) |
			(uint32(data[i+3]) << 24)
		i += 4

		k1 = ((k1&0xffff)*c1 + ((((k1 >> 16) * c1) & 0xffff) << 16)) & 0xffffffff
		k1 = (k1 << 15) | (k1 >> 17)
		k1 = ((k1&0xffff)*c2 + ((((k1 >> 16) * c2) & 0xffff) << 16)) & 0xffffffff

		h1 ^= k1
		h1 = (h1 << 13) | (h1 >> 19)
		h1b := ((h1&0xffff)*5 + ((((h1 >> 16) * 5) & 0xffff) << 16)) & 0xffffffff
		h1 = (h1b & 0xffff) + 0x6b64 + ((((h1b >> 16) + 0xe654) & 0xffff) << 16)
	}

	k1 := uint32(0)

	switch remainder {
	case 3:
		k1 ^= uint32(data[i+2]&0xff) << 16
		fallthrough
	case 2:
		k1 ^= uint32(data[i+1]&0xff) << 8
		fallthrough
	case 1:
		k1 ^= uint32(data[i] & 0xff)

		k1 = ((k1&0xffff)*c1 + ((((k1 >> 16) * c1) & 0xffff) << 16)) & 0xffffffff
		k1 = (k1 << 15) | (k1 >> 17)
		k1 = ((k1&0xffff)*c2 + ((((k1 >> 16) * c2) & 0xffff) << 16)) & 0xffffffff
		h1 ^= k1
	}

	h1 ^= uint32(length)

	h1 ^= h1 >> 16
	h1 = ((h1&0xffff)*0x85ebca6b + ((((h1 >> 16) * 0x85ebca6b) & 0xffff) << 16)) & 0xffffffff
	h1 ^= h1 >> 13
	h1 = ((h1&0xffff)*0xc2b2ae35 + ((((h1 >> 16) * 0xc2b2ae35) & 0xffff) << 16)) & 0xffffffff
	h1 ^= h1 >> 16

	return h1
}
