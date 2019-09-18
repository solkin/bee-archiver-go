package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"time"
)

type Leaf struct {
	Value     uint8
	Frequency int
	Zero      *Leaf
	One       *Leaf
	Bit       bool
	Parent    *Leaf
}

const BufferSize = 4096

func main() {
	fmt.Println("Bee Compress (Go)")

	source1 := "/Users/solkin/Desktop/apps-list.json"
	source2 := "/Users/solkin/Desktop/apps-list(2).json"
	output := "/Users/solkin/Desktop/apps-list.bzz"
	createArchive(source1, output)
	extractArchive(output, source2)
}

func extractArchive(source string, output string) {
	srcFile, err := os.Open(source)
	if err != nil {
		panic(err)
	}
	outFile, err := os.Create(output)
	if err != nil {
		panic(err)
	}
	reader := NewReader(srcFile)
	writer := NewWriter(outFile)

	tree, err := readDictionary(reader)
	if err != nil {
		panic(err)
	}

	size, err := readFileSize(reader)
	if err != nil {
		panic(err)
	}

	if err := decompress(tree, size, reader, writer); err != nil {
		panic(err)
	}

	err = srcFile.Close()
	if err != nil {
		panic(err)
	}
	err = outFile.Close()
	if err != nil {
		panic(err)
	}
}

func createArchive(source string, output string) {
	leafs, err := scan(source)
	if err != nil {
		panic(err)
	}

	tree := buildTree(leafs)
	dict := flatTree(tree, leafs)

	srcFile, err := os.Open(source)
	if err != nil {
		panic(err)
	}
	outFile, err := os.Create(output)
	if err != nil {
		panic(err)
	}
	reader := NewReader(srcFile)
	writer := NewWriter(outFile)

	err = writeDictionary(dict, len(leafs), writer)
	if err != nil {
		panic(err)
	}

	err = writeFileSize(srcFile, writer)
	if err != nil {
		panic(err)
	}

	err = compress(dict, reader, writer)
	if err != nil {
		panic(err)
	}

	err = srcFile.Close()
	if err != nil {
		panic(err)
	}
	err = outFile.Close()
	if err != nil {
		panic(err)
	}
}

func scan(path string) ([]*Leaf, error) {
	start := time.Now().UnixNano()

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	freqs := make([]int, 256)

	unique := 0

	buf := make([]byte, BufferSize)
	for {
		n, err := file.Read(buf)
		if err != nil {
			break
		}
		for i := 0; i < n; i++ {
			value := buf[i]
			if freqs[value] == 0 {
				unique++
			}
			freqs[value]++
		}
	}

	err = file.Close()
	if err != nil {
		return nil, err
	}

	leafs := make([]*Leaf, unique)
	l := 0
	for i := 0; i < len(freqs); i++ {
		freq := freqs[i]
		if freq > 0 {
			leafs[l] = &Leaf{
				Value:     uint8(i),
				Frequency: freq,
			}
			l++
		}
	}

	fmt.Printf("scan time: %d msec\n", (time.Now().UnixNano()-start)/1000000)

	return leafs, nil
}

func buildTree(leafs []*Leaf) []*Leaf {
	tree := make([]*Leaf, len(leafs))
	copy(tree, leafs)

	for len(tree) > 1 {
		sort.SliceStable(tree, func(i, j int) bool {
			return tree[i].Frequency < tree[j].Frequency
		})
		zero := tree[0]
		zero.Bit = false
		one := tree[1]
		one.Bit = true
		parent := &Leaf{
			Frequency: zero.Frequency + one.Frequency,
			Zero:      zero,
			One:       one,
		}
		zero.Parent = parent
		one.Parent = parent
		tree[1] = parent
		tree = tree[1:]
	}
	return tree
}

func flatTree(tree []*Leaf, leafs []*Leaf) [256][]bool {
	root := tree[0]
	var dict [256][]bool
	for _, leaf := range leafs {
		parent := leaf
		var path []bool
		for true {
			path = append(path, parent.Bit)
			parent = parent.Parent
			if parent == root {
				break
			}
		}
		dict[leaf.Value] = path
	}
	return dict
}

func readDictionary(reader Reader) (*Leaf, error) {
	var header struct {
		Version uint16
		Count   uint32
	}

	if err := binary.Read(reader, binary.BigEndian, &header); err != nil {
		return nil, err
	}

	if header.Version == 1 {
		leafs := make([]*Leaf, header.Count)
		var value uint8
		var frequency uint32
		for i := 0; i < int(header.Count); i++ {
			if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
				return nil, err
			}
			if err := binary.Read(reader, binary.BigEndian, &frequency); err != nil {
				return nil, err
			}
			leafs[i] = &Leaf{
				Value:     value,
				Frequency: int(frequency),
			}
		}
		return buildTree(leafs)[0], nil
	} else if header.Version == 2 {
		var sizes [256]uint8
		for i := 0; i < int(header.Count); i++ {
			var value uint8
			var size uint8
			if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
				return nil, err
			}
			if err := binary.Read(reader, binary.BigEndian, &size); err != nil {
				return nil, err
			}
			sizes[value] = size
		}
		root := &Leaf{}
		parent := root
		for i := 0; i < len(sizes); i++ {
			if size := sizes[i]; size > 0 {
				for c := 0; c < int(size); c++ {
					bit, err := reader.ReadBool()
					if err != nil {
						return nil, err
					}
					if bit {
						if parent.One == nil {
							parent.One = &Leaf{}
						}
						parent = parent.One
					} else {
						if parent.Zero == nil {
							parent.Zero = &Leaf{}
						}
						parent = parent.Zero
					}
				}
				parent.Value = uint8(i)
				parent = root
			}
		}
		return root, nil
	} else {
		panic(fmt.Sprintf("Unsupported archive verision %d", header.Version))
	}
}

func writeDictionary(dict [256][]bool, count int, writer Writer) error {
	body := new(bytes.Buffer)
	bitOutput := NewWriter(body)

	version := 2

	if err := binary.Write(writer, binary.BigEndian, uint16(version)); err != nil {
		return err
	}
	if err := binary.Write(writer, binary.BigEndian, uint32(count)); err != nil {
		return err
	}

	for value, path := range dict {
		size := len(path)
		if size == 0 {
			continue
		}
		if err := binary.Write(writer, binary.BigEndian, uint8(value)); err != nil {
			return err
		}
		if err := binary.Write(writer, binary.BigEndian, uint8(size)); err != nil {
			return err
		}
		for i := size - 1; i >= 0; i-- {
			if err := bitOutput.WriteBool(path[i]); err != nil {
				return err
			}
		}
	}
	if err := bitOutput.Close(); err != nil {
		return err
	}
	if _, err := writer.Write(body.Bytes()); err != nil {
		return err
	}

	return nil
}

func readFileSize(file Reader) (uint64, error) {
	var size uint64
	if err := binary.Read(file, binary.BigEndian, &size); err != nil {
		return 0, err
	}
	fmt.Println("size: ", size)
	return size, nil
}

func writeFileSize(srcFile *os.File, writer Writer) error {
	stat, err := srcFile.Stat()
	if err != nil {
		return err
	}
	size := uint64(stat.Size())
	if err = binary.Write(writer, binary.BigEndian, size); err != nil {
		return err
	}
	fmt.Println("size: ", size)
	return nil
}

func decompress(tree *Leaf, size uint64, reader Reader, writer Writer) error {
	start := time.Now().UnixNano()

	var written uint64
	root := tree
	var leaf = root
	for {
		b, err := reader.ReadBool()
		if err != nil {
			panic(err)
		}
		var child *Leaf
		if b {
			child = leaf.One
		} else {
			child = leaf.Zero
		}
		if child.Zero != nil || child.One != nil {
			leaf = child
		} else {
			if err := binary.Write(writer, binary.BigEndian, child.Value); err != nil {
				return err
			}
			leaf = root
			if written++; written == size {
				break
			}
		}
	}

	fmt.Printf("decompress time: %d msec\n", (time.Now().UnixNano()-start)/1000000)
	return nil
}

func compress(dict [256][]bool, reader Reader, writer Writer) error {
	start := time.Now().UnixNano()

	buf := make([]byte, BufferSize)
	for {
		n, err := reader.Read(buf)
		if err != nil {
			break
		}
		for i := 0; i < n; i++ {
			value := buf[i]
			path := dict[value]
			for j := len(path) - 1; j >= 0; j-- {
				if err := writer.WriteBool(path[j]); err != nil {
					panic(err)
				}
			}
		}
	}
	if _, err := writer.Align(); err != nil {
		panic(err)
	}
	if err := writer.Close(); err != nil {
		panic(err)
	}

	fmt.Printf("compress time: %d msec\n", (time.Now().UnixNano()-start)/1000000)
	return nil
}
