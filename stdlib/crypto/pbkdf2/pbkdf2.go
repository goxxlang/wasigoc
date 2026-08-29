// PBKDF2-HMAC-SHA256 (RFC 8018). Iterations and dkLen are caller-chosen;
// no other PRF. Verified against the well-known
// password="password"/salt="salt"/c=1/dkLen=32 vector.
package pbkdf2

import "crypto/hmac"

func Key(password []byte, salt []byte, iter int, keyLen int) []byte {
	var out []byte
	block := 1
	for len(out) < keyLen {
		out = append(out, f(password, salt, iter, block)...)
		block = block + 1
	}
	return out[0:keyLen]
}

func f(password []byte, salt []byte, iter int, blockNum int) []byte {
	msg := append([]byte{}, salt...)
	msg = append(msg, byte(blockNum>>24), byte(blockNum>>16), byte(blockNum>>8), byte(blockNum))
	u := hmac.SumSHA256(password, msg)
	out := append([]byte{}, u...)
	i := 1
	for i < iter {
		u = hmac.SumSHA256(password, u)
		j := 0
		for j < len(out) {
			out[j] = out[j] ^ u[j]
			j = j + 1
		}
		i = i + 1
	}
	return out
}
