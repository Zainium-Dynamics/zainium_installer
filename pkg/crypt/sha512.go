// Package crypt makes "$6$" SHA-512 crypt hashes (Drepper's spec), the format Zainium's
// shadow and pam_unix use.
package crypt

import (
	"crypto/rand"
	"crypto/sha512"
	"strconv"
	"strings"
)

const (
	alphabet      = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	defaultRounds = 5000
	maxSalt       = 16
)

// NewSalt returns a random 16-character salt.
func NewSalt() string {
	b := make([]byte, maxSalt)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}

// SHA512 hashes password with a fresh random salt.
func SHA512(password string) string { return SHA512Salt(password, "$6$"+NewSalt()) }

// SHA512Salt hashes with a "$6$[rounds=N$]salt" setting.
func SHA512Salt(password, setting string) string {
	s := strings.TrimPrefix(setting, "$6$")
	rounds, custom := defaultRounds, false
	if strings.HasPrefix(s, "rounds=") {
		if i := strings.IndexByte(s, '$'); i > 0 {
			if n, err := strconv.Atoi(s[len("rounds="):i]); err == nil {
				rounds, custom = min(max(n, 1000), 999999999), true
				s = s[i+1:]
			}
		}
	}
	if i := strings.IndexByte(s, '$'); i >= 0 {
		s = s[:i]
	}
	if len(s) > maxSalt {
		s = s[:maxSalt]
	}
	pw, salt := []byte(password), []byte(s)

	b := sha512.New()
	b.Write(pw)
	b.Write(salt)
	b.Write(pw)
	sumB := b.Sum(nil)

	a := sha512.New()
	a.Write(pw)
	a.Write(salt)
	for n := len(pw); n > 0; n -= 64 {
		a.Write(sumB[:min(n, 64)])
	}
	for n := len(pw); n > 0; n >>= 1 {
		if n&1 != 0 {
			a.Write(sumB)
		} else {
			a.Write(pw)
		}
	}
	sumA := a.Sum(nil)

	dp := sha512.New()
	for range pw {
		dp.Write(pw)
	}
	p := repeat(dp.Sum(nil), len(pw))

	ds := sha512.New()
	for i := 0; i < 16+int(sumA[0]); i++ {
		ds.Write(salt)
	}
	sb := repeat(ds.Sum(nil), len(salt))

	c := sumA
	for i := 0; i < rounds; i++ {
		h := sha512.New()
		if i&1 != 0 {
			h.Write(p)
		} else {
			h.Write(c)
		}
		if i%3 != 0 {
			h.Write(sb)
		}
		if i%7 != 0 {
			h.Write(p)
		}
		if i&1 != 0 {
			h.Write(c)
		} else {
			h.Write(p)
		}
		c = h.Sum(nil)
	}

	out := "$6$"
	if custom {
		out += "rounds=" + strconv.Itoa(rounds) + "$"
	}
	out += string(salt) + "$"
	order := [][3]int{
		{0, 21, 42}, {22, 43, 1}, {44, 2, 23}, {3, 24, 45}, {25, 46, 4}, {47, 5, 26}, {6, 27, 48},
		{28, 49, 7}, {50, 8, 29}, {9, 30, 51}, {31, 52, 10}, {53, 11, 32}, {12, 33, 54}, {34, 55, 13},
		{56, 14, 35}, {15, 36, 57}, {37, 58, 16}, {59, 17, 38}, {18, 39, 60}, {40, 61, 19}, {62, 20, 41},
	}
	for _, o := range order {
		out += b64(uint(c[o[0]])<<16|uint(c[o[1]])<<8|uint(c[o[2]]), 4)
	}
	out += b64(uint(c[63]), 2)
	return out
}

func repeat(sum []byte, n int) []byte {
	out := make([]byte, 0, n)
	for len(out) < n {
		out = append(out, sum[:min(n-len(out), len(sum))]...)
	}
	return out
}

func b64(v uint, n int) string {
	var sb strings.Builder
	for ; n > 0; n-- {
		sb.WriteByte(alphabet[v&0x3f])
		v >>= 6
	}
	return sb.String()
}
