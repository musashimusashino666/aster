package bytecode

import (
	"encoding/json"
	"testing"
)

func TestArrayRuntimeMetadataDoesNotChangeBytecodeJSON(t *testing.T) {
	value := Array([]Value{
		I64(1),
		Array([]Value{I64(2)}),
	})
	value.MarkArrayShared()
	value.SetArrayBorrowed(true)

	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"kind":"array","array":[{"kind":"i64","i64":1},{"kind":"array","array":[{"kind":"i64","i64":2}]}]}`
	if got := string(encoded); got != want {
		t.Fatalf("JSON representation changed: got %s, want %s", got, want)
	}

	var decoded Value
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.arrayStorage != nil || decoded.arrayBorrowed {
		t.Fatal("runtime COW metadata must not be serialized")
	}
}

func TestArrayPrepareWriteDetachesSharedNestedStorage(t *testing.T) {
	original := Array([]Value{
		Array([]Value{I64(1), I64(2)}),
		I64(3),
	})
	copied := original
	copied.MarkArrayShared()
	copied.PrepareArrayWrite()

	if len(original.Array) != 2 || len(copied.Array) != 2 {
		t.Fatal("unexpected outer array length")
	}
	if &original.Array[0] == &copied.Array[0] {
		t.Fatal("shared outer storage was not detached")
	}

	originalChild := original.Array[0]
	copiedChild := copied.Array[0]
	if !originalChild.arrayStorage.shared || !copiedChild.arrayStorage.shared {
		t.Fatal("detaching an outer array must mark nested array storage shared")
	}
	copiedChild.PrepareArrayWrite()
	copiedChild.Array[0] = I64(9)
	copied.Array[0] = copiedChild

	if got := original.Array[0].StringValue(); got != "[1 2]" {
		t.Fatalf("nested value changed through original alias: got %q", got)
	}
	if got := copied.Array[0].StringValue(); got != "[9 2]" {
		t.Fatalf("unexpected detached nested value: got %q", got)
	}
}
