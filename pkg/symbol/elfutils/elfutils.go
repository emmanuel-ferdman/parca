// Copyright 2022 The Parca Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package elfutils

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/nanmu42/limitio"
)

var dwarfSuffix = func(s *elf.Section) string {
	switch {
	case strings.HasPrefix(s.Name, ".debug_"):
		return s.Name[7:]
	case strings.HasPrefix(s.Name, ".zdebug_"):
		return s.Name[8:]
	case strings.HasPrefix(s.Name, "__debug_"): // macos
		return s.Name[8:]
	default:
		return ""
	}
}

// HasDWARF reports whether the specified executable or library file contains DWARF debug information.
func HasDWARF(path string) (bool, error) {
	f, err := elf.Open(path)
	if err != nil {
		return false, fmt.Errorf("failed to open elf: %w", err)
	}
	defer f.Close()

	sections, err := readableDWARFSections(f)
	if err != nil {
		return false, fmt.Errorf("failed to read DWARF sections: %w", err)
	}

	return len(sections) > 0, nil
}

// A simplified and modified version of debug/elf.DWARF().
func readableDWARFSections(f *elf.File) (map[string]struct{}, error) {
	// There are many DWARf sections, but these are the ones
	// the debug/dwarf package started with "abbrev", "info", "str", "line", "ranges".
	// Possible candidates for future: "loc", "loclists", "rnglists"
	sections := map[string]*string{"abbrev": nil, "info": nil, "str": nil, "line": nil, "ranges": nil}
	exists := map[string]struct{}{}
	for _, s := range f.Sections {
		suffix := dwarfSuffix(s)
		if suffix == "" {
			continue
		}
		if _, ok := sections[suffix]; !ok {
			continue
		}
		if s.Type == elf.SHT_PROGBITS {
			exists[suffix] = struct{}{}
		}
	}

	return exists, nil
}

// IsSymbolizableGoObjFile checks whether the specified executable or library file is generated by Go toolchain
// and has necessary symbol information attached.
func IsSymbolizableGoObjFile(path string) (bool, error) {
	// Checks ".note.go.buildid" section and symtab better to keep those sections in object file.
	f, err := elf.Open(path)
	if err != nil {
		return false, fmt.Errorf("failed to open elf: %w", err)
	}
	defer f.Close()

	isGo := false
	for _, s := range f.Sections {
		if s.Name == ".note.go.buildid" {
			isGo = true
		}
	}

	// In case ".note.go.buildid" section is stripped, check for symbols.
	if !isGo {
		syms, err := f.Symbols()
		if err != nil {
			return false, fmt.Errorf("failed to read symbols: %w", err)
		}
		for _, sym := range syms {
			name := sym.Name
			if name == "runtime.main" || name == "main.main" {
				isGo = true
			}
			if name == "runtime.buildVersion" {
				isGo = true
			}
		}
	}

	if !isGo {
		return false, nil
	}

	// Check if the Go binary symbolizable.
	// Go binaries has a special case. They use ".gopclntab" section to symbolize addresses.
	if sec := f.Section(".gopclntab"); sec != nil {
		if sec.Type == elf.SHT_PROGBITS {
			return true, nil
		}
	}

	return false, errors.New("failed to detect .gopclntab section or section has no bits")
}

// IsGoObjFile checks whether the specified executable or library file is generated by Go toolchain.
func IsGoObjFile(path string) (bool, error) {
	// Checks ".note.go.buildid" section and symtab better to keep those sections in object file.
	f, err := elf.Open(path)
	if err != nil {
		return false, fmt.Errorf("failed to open elf: %w", err)
	}
	defer f.Close()

	for _, s := range f.Sections {
		if s.Name == ".note.go.buildid" {
			return true, nil
		}
	}
	return false, nil
}

// HasSymbols reports whether the specified executable or library file contains symbols (both.symtab and .dynsym).
func HasSymbols(path string) (bool, error) {
	ef, err := elf.Open(path)
	if err != nil {
		return false, fmt.Errorf("failed to open elf: %w", err)
	}
	defer ef.Close()

	for _, section := range ef.Sections {
		if section.Type == elf.SHT_SYMTAB || section.Type == elf.SHT_DYNSYM {
			return true, nil
		}
	}
	return false, nil
}

// ValidateFile returns an error if the given object file is not valid.
func ValidateFile(path string) error {
	elfFile, err := elf.Open(path)
	if err != nil {
		return err
	}
	defer elfFile.Close()

	// TODO(kakkoyun): How can we improve this without allocating too much memory.
	if len(elfFile.Sections) == 0 {
		return errors.New("ELF does not have any sections")
	}
	return nil
}

// ValidateHeader returns an error if the given object file header is not valid.
func ValidateHeader(r io.Reader) error {
	// Identity reader.
	buf := bytes.NewBuffer(nil)
	// limitio.Writer is used to avoid buffer overflow.
	// We only need to read the first 2 bytes.
	// If we receive a longer data, we will ignore the rest without an error.
	w := limitio.NewWriter(buf, 16, true)

	// NOTICE: The ELF header is 52 or 64 bytes long for 32-bit and 64-bit binaries respectively
	r = io.TeeReader(io.LimitReader(r, 64), w)

	// We need to read the entire header to determine the class of the file.
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	r = bytes.NewReader(b)

	var ident [16]byte
	_, err = buf.Read(ident[:])
	if err != nil {
		return err
	}
	if ident[0] != '\x7f' || ident[1] != 'E' || ident[2] != 'L' || ident[3] != 'F' {
		return fmt.Errorf("invalid magic number, %s", ident[0:4])
	}

	c := elf.Class(ident[elf.EI_CLASS])
	switch c {
	case elf.ELFCLASS32:
	case elf.ELFCLASS64:
		// ok
	default:
		return fmt.Errorf("unknown ELF class, %s", c)
	}

	var byteOrder binary.ByteOrder
	d := elf.Data(ident[elf.EI_DATA])
	switch d {
	case elf.ELFDATA2LSB:
		byteOrder = binary.LittleEndian
	case elf.ELFDATA2MSB:
		byteOrder = binary.BigEndian
	default:
		return fmt.Errorf("unknown ELF data encoding, %s", d)
	}

	fv := elf.Version(ident[elf.EI_VERSION])
	if fv != elf.EV_CURRENT {
		return fmt.Errorf("unknown ELF version, %s", fv)
	}

	// Read ELF file header.
	var shoff int64
	var shnum, shstrndx int
	switch c {
	case elf.ELFCLASS32:
		hdr := new(elf.Header32)
		if err := binary.Read(io.LimitReader(r, 52), byteOrder, hdr); err != nil {
			return err
		}
		if v := elf.Version(hdr.Version); v != fv {
			return fmt.Errorf("invalid ELF version, %s", v)
		}
		shoff = int64(hdr.Shoff)
		shnum = int(hdr.Shnum)
		shstrndx = int(hdr.Shstrndx)
	case elf.ELFCLASS64:
		hdr := new(elf.Header64)
		if err := binary.Read(r, byteOrder, hdr); err != nil {
			return err
		}
		if v := elf.Version(hdr.Version); v != fv {
			return fmt.Errorf("invalid ELF version, %s", v)
		}
		shoff = int64(hdr.Shoff)
		shnum = int(hdr.Shnum)
		shstrndx = int(hdr.Shstrndx)
	}

	if shoff == 0 && shnum != 0 {
		return fmt.Errorf("invalid ELF file, shoff is 0 but shnum is %d", shnum)
	}

	if shnum > 0 && shstrndx >= shnum {
		return fmt.Errorf("invalid ELF file, shstrndx is %d but shnum is %d", shstrndx, shnum)
	}

	if shnum <= 0 {
		return fmt.Errorf("elf file has no sections")
	}

	return nil
}
