package sdk

import (
	"github.com/spaolacci/murmur3"
)

const (
	HASH_SEED           = 1
	MAX_HASH_VALUE      = 1 << 32
	MAX_BUCKETED_NUMBER = 100000 // 100% * 1000 to include three decimal places in the same integer value
)

func getBucketedNumber(bucketKey string) int {
	hashValue := murmur3.Sum32WithSeed([]byte(bucketKey), HASH_SEED)
	ratio := float64(hashValue) / float64(MAX_HASH_VALUE)

	return int(ratio * float64(MAX_BUCKETED_NUMBER))
}
