package crypt

import "testing"

// Vectors from Drepper's SHA-crypt specification.
func TestSHA512Vectors(t *testing.T) {
	cases := []struct{ setting, pw, want string }{
		{"$6$saltstring", "Hello world!",
			"$6$saltstring$svn8UoSVapNtMuq1ukKS4tPQd8iKwSMHWjl/O817G3uBnIFNjnQJuesI68u4OTLiBFdcbYEdFCoEOfaS35inz1"},
		{"$6$rounds=10000$saltstringsaltstring", "Hello world!",
			"$6$rounds=10000$saltstringsaltst$OW1/O6BYHV6BcXZu8QVeXbDWra3Oeqh0sbHbbMCVNSnCM/UrjmM0Dp8vOuZeHBy/YTBmSK6H9qs/y3RnOaw5v."},
		{"$6$rounds=5000$toolongsaltstring", "This is just a test",
			"$6$rounds=5000$toolongsaltstrin$lQ8jolhgVRVhY4b5pZKaysCLi0QBxGoNeKQzQ3glMhwllF7oGDZxUhx1yxdYcz/e1JSbq3y6JMxxl8audkUEm0"},
	}
	for _, c := range cases {
		if got := SHA512Salt(c.pw, c.setting); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.setting, got, c.want)
		}
	}
}

func TestRandomSaltRoundTrip(t *testing.T) {
	h := SHA512("secret")
	if SHA512Salt("secret", h) != h {
		t.Fatal("re-hashing with the stored setting must give the same hash")
	}
}
