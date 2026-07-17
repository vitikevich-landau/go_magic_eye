// Package guild — «чужой» пакет: все поля неэкспортированы.
package guild

// Member прячет всё: снаружи пакета к полям доступа нет.
type Member struct {
	name   string
	level  int16
	mana   float32
	secret [3]byte
}

// NewMember — единственная дверь.
func NewMember(name string, level int16) *Member {
	return &Member{name: name, level: level, mana: 42.5, secret: [3]byte{0xde, 0xad, 0x01}}
}
