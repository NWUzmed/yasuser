package types

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestURL(t *testing.T) {
	n := time.Now()
	u := &URL{
		Ori:    "https://kfd.me",
		Short:  "1B",
		Expire: &n,
		Pass:   "pass",
	}

	b := u.Bytes()
	lb := len(b)

	nu := new(URL)
	nu.Decode(b)

	assert.Equal(t, b[lb/2], nu.Bytes()[lb/2])
	assert.Equal(t, nu.Expire.Local(), u.Expire.Local())
	assert.Equal(t, nu.Ori, u.Ori)
}

func BenchmarkURL(b *testing.B) {
	n := time.Now()
	for i := 0; i < b.N; i++ {
		u := &URL{
			Ori:    "https://kfd.me",
			Short:  "1B",
			Expire: &n,
			Pass:   "pass",
		}

		bs := u.Bytes()
		nu := new(URL)
		nu.Decode(bs)
	}
}
