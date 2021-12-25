package cache

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

type testCache interface {
	Usage() (int, int)
}

func verifyEmptyCache(c testCache, t *testing.T) {
	o, s := c.Usage()
	if o != 0 || s != 0 {
		t.Errorf("cache is not empty: objects %v, shields %v", o, s)
	}
}

func TestShieldedCacheTTL(t *testing.T) {
	require := require.New(t)

	ctx, ctxCancel := context.WithCancel(context.Background())

	c := NewShieldedCache[uint64](time.Millisecond * 100)

	defer func() {
		ctxCancel()
		goleak.VerifyNone(t)
		verifyEmptyCache(c, t)
	}()

	require.NoError(c.StartWorker(ctx))

	var u uint64

	fetchFunc := func() (uint64, error) {
		u++
		if u > 2 {
			return 0, fmt.Errorf("error")
		}
		return u, nil
	}

	res, hit, err := c.Fetch("test", time.Second*2, fetchFunc)
	require.False(hit)
	require.NoError(err)
	require.Equal(res.Data, uint64(1))

	res, hit, err = c.Fetch("test", time.Second*2, fetchFunc)
	require.True(hit)
	require.NoError(err)
	require.Equal(res.Data, uint64(1))

	time.Sleep(time.Second)

	res, hit, err = c.Fetch("test", time.Second*2, fetchFunc)
	require.True(hit)
	require.NoError(err)
	require.Equal(res.Data, uint64(1))

	time.Sleep(time.Second * 2)

	res, hit, err = c.Fetch("test", time.Second*2, fetchFunc)
	require.False(hit)
	require.NoError(err)
	require.Equal(res.Data, uint64(2))

	time.Sleep(time.Second * 1)

	res, hit, err = c.Fetch("test", time.Second*2, fetchFunc)
	require.True(hit)
	require.NoError(err)
	require.Equal(res.Data, uint64(2))

	time.Sleep(time.Second * 2)

	res, hit, err = c.Fetch("test", time.Second*2, fetchFunc)
	require.False(hit)
	require.Error(err)

	time.Sleep(time.Second * 1)

	res, hit, err = c.Fetch("test", time.Second*2, fetchFunc)
	require.False(hit)
	require.Error(err)
}

func TestShieldedCacheShielding(t *testing.T) {
	require := require.New(t)

	ctx, ctxCancel := context.WithCancel(context.Background())

	defer func() {
		ctxCancel()
		goleak.VerifyNone(t)
	}()

	c := NewShieldedCache[uint64](time.Millisecond * 100)

	require.NoError(c.StartWorker(ctx))

	var fetching bool
	var n uint64

	fetch := func() (uint64, error) {
		require.False(fetching, "duplicate fetch")
		fetching = true
		defer func() {
			fetching = false
		}()

		time.Sleep(time.Millisecond * 300)
		n++
		return n, nil
	}

	for i := 0; i < 5; i++ {
		go func() {
			for {
				time.Sleep(time.Millisecond * 10)
				select {
				case <-ctx.Done():
					return
				default:
				}
				_, _, err := c.Fetch("testkey", time.Millisecond*50, fetch)
				require.NoError(err)
			}
		}()
	}

	time.Sleep(time.Millisecond*10*10*(5+1) + time.Millisecond*(300*10+50*10))
	require.Equal(uint64(10), n)
}
