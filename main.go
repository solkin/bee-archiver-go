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

	source := "/Users/solkin/Desktop/apps-list.json"
	output := "/Users/solkin/Desktop/apps-list.bee"
	createArchive(source, output)
	extractArchive(output, source)
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

	tree, err := readDictionary(srcFile)
	if err != nil {
		panic(err)
	}
	fmt.Println("tree: ", len(tree))

	size, err := readFileSize(srcFile)
	if err != nil {
		panic(err)
	}
	fmt.Println("size: ", size)

	if err := decompress(tree, size, srcFile, outFile); err != nil {
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

	//err = writeDictionary2(dict, len(leafs), outFile)
	err = writeDictionary(leafs, outFile)
	if err != nil {
		panic(err)
	}

	err = writeFileSize(srcFile, outFile)
	if err != nil {
		panic(err)
	}

	err = compress(dict, srcFile, outFile)
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

func readDictionary(file *os.File) ([]*Leaf, error) {
	var header struct {
		Version uint16
		Count   uint32
	}

	if err := binary.Read(file, binary.BigEndian, &header); err != nil {
		return nil, err
	}

	leafs := make([]*Leaf, header.Count)

	var value uint8
	var frequency uint32
	for i := 0; i < int(header.Count); i++ {
		if err := binary.Read(file, binary.BigEndian, &value); err != nil {
			return nil, err
		}
		if err := binary.Read(file, binary.BigEndian, &frequency); err != nil {
			return nil, err
		}
		leafs[i] = &Leaf{
			Value:     value,
			Frequency: int(frequency),
		}
	}

	tree := buildTree(leafs)

	return tree, nil
}

func writeDictionary(leafs []*Leaf, file *os.File) error {
	buf := new(bytes.Buffer)

	version := 1
	count := len(leafs)

	header := []interface{}{
		uint16(version),
		uint32(count),
	}

	for _, v := range header {
		err := binary.Write(buf, binary.BigEndian, v)
		if err != nil {
			return err
		}
	}
	for _, leaf := range leafs {
		if err := binary.Write(buf, binary.BigEndian, leaf.Value); err != nil {
			return err
		}
		if err := binary.Write(buf, binary.BigEndian, uint32(leaf.Frequency)); err != nil {
			return err
		}
	}
	if _, err := file.Write(buf.Bytes()); err != nil {
		return err
	}

	return nil
}

func readFileSize(file *os.File) (uint64, error) {
	var size uint64
	if err := binary.Read(file, binary.BigEndian, &size); err != nil {
		return 0, err
	}
	return size, nil
}

func writeFileSize(srcFile *os.File, outFile *os.File) error {
	stat, err := srcFile.Stat()
	if err != nil {
		return err
	}

	buf := new(bytes.Buffer)

	if err = binary.Write(buf, binary.BigEndian, uint64(stat.Size())); err != nil {
		return err
	}

	if _, err = outFile.Write(buf.Bytes()); err != nil {
		return err
	}

	return nil
}

func decompress(tree []*Leaf, size uint64, source *os.File, output *os.File) error {
	start := time.Now().UnixNano()

	var written uint64
	root := tree[0]
	var leaf = root
	bitInput := NewReader(source)
	for {
		b, err := bitInput.ReadBool()
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
			if err := binary.Write(output, binary.BigEndian, child.Value); err != nil {
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

func compress(dict [256][]bool, source *os.File, output *os.File) error {
	start := time.Now().UnixNano()

	bitOutput := NewWriter(output)
	buf := make([]byte, BufferSize)
	for {
		n, err := source.Read(buf)
		if err != nil {
			break
		}
		for i := 0; i < n; i++ {
			value := buf[i]
			path := dict[value]
			for j := len(path) - 1; j >= 0; j-- {
				if err := bitOutput.WriteBool(path[j]); err != nil {
					panic(err)
				}
			}
		}
	}
	if _, err := bitOutput.Align(); err != nil {
		panic(err)
	}
	if err := bitOutput.Close(); err != nil {
		panic(err)
	}

	fmt.Printf("compress time: %d msec\n", (time.Now().UnixNano()-start)/1000000)
	return nil
}
