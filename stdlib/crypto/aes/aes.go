// Bounded AES: AES-128 only (16-byte key) -- real Go's crypto/aes also
// supports 192/256-bit keys (24/32 bytes), not implemented here. FIPS 197,
// textbook state-array formulation (SubBytes/ShiftRows/MixColumns/
// AddRoundKey), not a hardware-AES-NI or bitsliced/constant-time
// implementation -- same "correct, not hardened against timing side
// channels" caveat real Go's *pure Go* fallback path already carries
// (real Go prefers AES-NI/ARMv8 crypto extensions when available; this
// project has neither).
//
// Implements crypto/cipher's Block interface: BlockSize()==16,
// Encrypt/Decrypt operate on exactly one 16-byte block (no chaining mode).
package aes

import "errors"

const blockSize = 16
const nb = 4
const nk = 4
const nr = 10

var sbox = []byte{
	99, 124, 119, 123, 242, 107, 111, 197, 48, 1, 103, 43, 254, 215, 171, 118,
	202, 130, 201, 125, 250, 89, 71, 240, 173, 212, 162, 175, 156, 164, 114, 192,
	183, 253, 147, 38, 54, 63, 247, 204, 52, 165, 229, 241, 113, 216, 49, 21,
	4, 199, 35, 195, 24, 150, 5, 154, 7, 18, 128, 226, 235, 39, 178, 117,
	9, 131, 44, 26, 27, 110, 90, 160, 82, 59, 214, 179, 41, 227, 47, 132,
	83, 209, 0, 237, 32, 252, 177, 91, 106, 203, 190, 57, 74, 76, 88, 207,
	208, 239, 170, 251, 67, 77, 51, 133, 69, 249, 2, 127, 80, 60, 159, 168,
	81, 163, 64, 143, 146, 157, 56, 245, 188, 182, 218, 33, 16, 255, 243, 210,
	205, 12, 19, 236, 95, 151, 68, 23, 196, 167, 126, 61, 100, 93, 25, 115,
	96, 129, 79, 220, 34, 42, 144, 136, 70, 238, 184, 20, 222, 94, 11, 219,
	224, 50, 58, 10, 73, 6, 36, 92, 194, 211, 172, 98, 145, 149, 228, 121,
	231, 200, 55, 109, 141, 213, 78, 169, 108, 86, 244, 234, 101, 122, 174, 8,
	186, 120, 37, 46, 28, 166, 180, 198, 232, 221, 116, 31, 75, 189, 139, 138,
	112, 62, 181, 102, 72, 3, 246, 14, 97, 53, 87, 185, 134, 193, 29, 158,
	225, 248, 152, 17, 105, 217, 142, 148, 155, 30, 135, 233, 206, 85, 40, 223,
	140, 161, 137, 13, 191, 230, 66, 104, 65, 153, 45, 15, 176, 84, 187, 22,
}

var invSbox = []byte{
	82, 9, 106, 213, 48, 54, 165, 56, 191, 64, 163, 158, 129, 243, 215, 251,
	124, 227, 57, 130, 155, 47, 255, 135, 52, 142, 67, 68, 196, 222, 233, 203,
	84, 123, 148, 50, 166, 194, 35, 61, 238, 76, 149, 11, 66, 250, 195, 78,
	8, 46, 161, 102, 40, 217, 36, 178, 118, 91, 162, 73, 109, 139, 209, 37,
	114, 248, 246, 100, 134, 104, 152, 22, 212, 164, 92, 204, 93, 101, 182, 146,
	108, 112, 72, 80, 253, 237, 185, 218, 94, 21, 70, 87, 167, 141, 157, 132,
	144, 216, 171, 0, 140, 188, 211, 10, 247, 228, 88, 5, 184, 179, 69, 6,
	208, 44, 30, 143, 202, 63, 15, 2, 193, 175, 189, 3, 1, 19, 138, 107,
	58, 145, 17, 65, 79, 103, 220, 234, 151, 242, 207, 206, 240, 180, 230, 115,
	150, 172, 116, 34, 231, 173, 53, 133, 226, 249, 55, 232, 28, 117, 223, 110,
	71, 241, 26, 113, 29, 41, 197, 137, 111, 183, 98, 14, 170, 24, 190, 27,
	252, 86, 62, 75, 198, 210, 121, 32, 154, 219, 192, 254, 120, 205, 90, 244,
	31, 221, 168, 51, 136, 7, 199, 49, 177, 18, 16, 89, 39, 128, 236, 95,
	96, 81, 127, 169, 25, 181, 74, 13, 45, 229, 122, 159, 147, 201, 156, 239,
	160, 224, 59, 77, 174, 42, 245, 176, 200, 235, 187, 60, 131, 83, 153, 97,
	23, 43, 4, 126, 186, 119, 214, 38, 225, 105, 20, 99, 85, 33, 12, 125,
}

var rcon = []byte{0x01, 0x02, 0x04, 0x08, 0x10, 0x20, 0x40, 0x80, 0x1B, 0x36}

func gmul(a byte, b byte) byte {
	var p byte
	for i := 0; i < 8; i++ {
		if b&1 != 0 {
			p = p ^ a
		}
		hi := a & 0x80
		a = a << 1
		if hi != 0 {
			a = a ^ 0x1B
		}
		b = b >> 1
	}
	return p
}

type Cipher struct {
	roundKeys []byte
}

func expandKey(key []byte) []byte {
	w := make([]byte, nb*(nr+1)*4)
	for i := 0; i < nk*4; i++ {
		w[i] = key[i]
	}
	for i := nk; i < nb*(nr+1); i++ {
		var t0, t1, t2, t3 byte
		t0 = w[(i-1)*4+0]
		t1 = w[(i-1)*4+1]
		t2 = w[(i-1)*4+2]
		t3 = w[(i-1)*4+3]
		if i%nk == 0 {
			r0 := sbox[t1]
			r1 := sbox[t2]
			r2 := sbox[t3]
			r3 := sbox[t0]
			t0 = r0 ^ rcon[i/nk-1]
			t1 = r1
			t2 = r2
			t3 = r3
		}
		w[i*4+0] = w[(i-nk)*4+0] ^ t0
		w[i*4+1] = w[(i-nk)*4+1] ^ t1
		w[i*4+2] = w[(i-nk)*4+2] ^ t2
		w[i*4+3] = w[(i-nk)*4+3] ^ t3
	}
	return w
}

func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != 16 {
		return nil, errors.New("crypto/aes: invalid key size, only AES-128 (16-byte key) is supported")
	}
	return &Cipher{roundKeys: expandKey(key)}, nil
}

func (c *Cipher) BlockSize() int {
	return blockSize
}

func addRoundKey(state []byte, roundKeys []byte, round int) {
	for i := 0; i < 16; i++ {
		state[i] = state[i] ^ roundKeys[round*16+i]
	}
}

func subBytes(state []byte, box []byte) {
	for i := 0; i < 16; i++ {
		state[i] = box[state[i]]
	}
}

func shiftRows(state []byte) {
	out := make([]byte, 16)
	for r := 0; r < 4; r++ {
		for c := 0; c < 4; c++ {
			out[r+4*c] = state[r+4*((c+r)%4)]
		}
	}
	for i := 0; i < 16; i++ {
		state[i] = out[i]
	}
}

func invShiftRows(state []byte) {
	out := make([]byte, 16)
	for r := 0; r < 4; r++ {
		for c := 0; c < 4; c++ {
			out[r+4*((c+r)%4)] = state[r+4*c]
		}
	}
	for i := 0; i < 16; i++ {
		state[i] = out[i]
	}
}

func mixColumns(state []byte) {
	for c := 0; c < 4; c++ {
		a0 := state[4*c+0]
		a1 := state[4*c+1]
		a2 := state[4*c+2]
		a3 := state[4*c+3]
		state[4*c+0] = gmul(a0, 2) ^ gmul(a1, 3) ^ a2 ^ a3
		state[4*c+1] = a0 ^ gmul(a1, 2) ^ gmul(a2, 3) ^ a3
		state[4*c+2] = a0 ^ a1 ^ gmul(a2, 2) ^ gmul(a3, 3)
		state[4*c+3] = gmul(a0, 3) ^ a1 ^ a2 ^ gmul(a3, 2)
	}
}

func invMixColumns(state []byte) {
	for c := 0; c < 4; c++ {
		a0 := state[4*c+0]
		a1 := state[4*c+1]
		a2 := state[4*c+2]
		a3 := state[4*c+3]
		state[4*c+0] = gmul(a0, 14) ^ gmul(a1, 11) ^ gmul(a2, 13) ^ gmul(a3, 9)
		state[4*c+1] = gmul(a0, 9) ^ gmul(a1, 14) ^ gmul(a2, 11) ^ gmul(a3, 13)
		state[4*c+2] = gmul(a0, 13) ^ gmul(a1, 9) ^ gmul(a2, 14) ^ gmul(a3, 11)
		state[4*c+3] = gmul(a0, 11) ^ gmul(a1, 13) ^ gmul(a2, 9) ^ gmul(a3, 14)
	}
}

func (c *Cipher) Encrypt(dst []byte, src []byte) {
	state := make([]byte, 16)
	for i := 0; i < 16; i++ {
		state[i] = src[i]
	}
	addRoundKey(state, c.roundKeys, 0)
	for round := 1; round < nr; round++ {
		subBytes(state, sbox)
		shiftRows(state)
		mixColumns(state)
		addRoundKey(state, c.roundKeys, round)
	}
	subBytes(state, sbox)
	shiftRows(state)
	addRoundKey(state, c.roundKeys, nr)
	for i := 0; i < 16; i++ {
		dst[i] = state[i]
	}
}

func (c *Cipher) Decrypt(dst []byte, src []byte) {
	state := make([]byte, 16)
	for i := 0; i < 16; i++ {
		state[i] = src[i]
	}
	addRoundKey(state, c.roundKeys, nr)
	for round := nr - 1; round > 0; round-- {
		invShiftRows(state)
		subBytes(state, invSbox)
		addRoundKey(state, c.roundKeys, round)
		invMixColumns(state)
	}
	invShiftRows(state)
	subBytes(state, invSbox)
	addRoundKey(state, c.roundKeys, 0)
	for i := 0; i < 16; i++ {
		dst[i] = state[i]
	}
}
