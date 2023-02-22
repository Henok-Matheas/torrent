package bitfield

// A Bitfield represents the pieces that the owner of the Bitfield has
type Bitfield []byte

// HasPiece tells if a bitfield has a particular piece index set
func (bf Bitfield) HasPiece(index int) bool {
	byteIndex := index / 8
	offset := index % 8
	if byteIndex < 0 || byteIndex >= len(bf) {
		return false
	}
	return bf[byteIndex]>>uint(7-offset)&1 != 0
}

// SetPiece sets a bit in the bitfield
func (bf Bitfield) SetPiece(index int) {
	byteIndex := index / 8
	offset := index % 8

	bf[byteIndex] |= 1 << uint(7-offset)
}
