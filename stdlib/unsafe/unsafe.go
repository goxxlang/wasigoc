// Tiny unsafe: Pointer is a uintptr alias (not a C++ void*, and not
// real Go's Pointer which the compiler itself understands). Add does
// uintptr arithmetic. Sizeof/Alignof/Offsetof are compiler builtins in
// real Go and are NOT implemented here -- they would need wasigoc to
// emit sizeof(T), a bigger feature than this package. Keep tiny, same
// as the tracker line said.
package unsafe

type Pointer uint64

func Add(ptr Pointer, offset int) Pointer {
	return Pointer(uint64(ptr) + uint64(offset))
}

func PointerFromInt(n int) Pointer {
	return Pointer(uint64(n))
}

func IntFromPointer(p Pointer) int {
	return int(uint64(p))
}
