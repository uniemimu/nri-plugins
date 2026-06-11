// Copyright 2019 Intel Corporation. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sysfs

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/containers/nri-plugins/pkg/utils/cpuset"
	idset "github.com/intel/goresctrl/pkg/utils"
)

// Get the trailing enumeration part of a name.
func getEnumeratedID(name string) idset.ID {
	id := 0
	base := 1
	for idx := len(name) - 1; idx > 0; idx-- {
		d := name[idx]

		if '0' <= d && d <= '9' {
			id += base * (int(d) - '0')
			base *= 10
		} else {
			if base > 1 {
				return idset.ID(id)
			}

			return idset.ID(-1)
		}
	}

	return idset.ID(-1)
}

// signedInt is the type constraint for signed integer types that can be read
// from sysfs, including named types whose underlying type is a signed integer
// (e.g. idset.ID).
type signedInt interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64
}

// unsignedInt is the type constraint for unsigned integer types that can be
// read from sysfs.
type unsignedInt interface {
	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

// readSysfsRaw reads and trims the newline-terminated content of base/entry.
func readSysfsRaw(base, entry string) (string, error) {
	path := filepath.Join(base, entry)
	blob, err := os.ReadFile(path)
	if err != nil {
		return "", sysfsError(path, "failed to read sysfs entry: %w", err)
	}
	return strings.Trim(string(blob), "\n"), nil
}

// readSysfsInt reads a sysfs file and parses it as a signed integer of type T.
// T may be any named type whose underlying type is a signed integer (e.g. idset.ID).
func readSysfsInt[T signedInt](base, entry string, dst *T) error {
	buf, err := readSysfsRaw(base, entry)
	if err != nil {
		return err
	}
	v, err := parseIntTo[T](buf)
	if err != nil {
		return sysfsError(filepath.Join(base, entry), "invalid integer %q: %w", buf, err)
	}
	*dst = v
	return nil
}

// readSysfsUint reads a sysfs file and parses it as an unsigned integer of type T.
func readSysfsUint[T unsignedInt](base, entry string, dst *T) error {
	buf, err := readSysfsRaw(base, entry)
	if err != nil {
		return err
	}
	v, err := parseUintTo[T](buf)
	if err != nil {
		return sysfsError(filepath.Join(base, entry), "invalid unsigned integer %q: %w", buf, err)
	}
	*dst = v
	return nil
}

// readSysfsString reads a sysfs file and stores the trimmed content in dst.
func readSysfsString(base, entry string, dst *string) error {
	buf, err := readSysfsRaw(base, entry)
	if err != nil {
		return err
	}
	*dst = buf
	return nil
}

// readSysfsIDSet reads a sysfs file and parses it as a sep-delimited IDSet.
func readSysfsIDSet(base, entry, sep string, dst *idset.IDSet) error {
	buf, err := readSysfsRaw(base, entry)
	if err != nil {
		return err
	}
	if err := parseIDSet(buf, sep, dst); err != nil {
		return sysfsError(filepath.Join(base, entry), "%w", err)
	}
	return nil
}

// readSysfsIntList reads a sysfs file as a sep-delimited list of signed integers.
func readSysfsIntList[T signedInt](base, entry, sep string, dst *[]T) error {
	buf, err := readSysfsRaw(base, entry)
	if err != nil {
		return err
	}
	v, err := parseIntSlice[T](buf, sep)
	if err != nil {
		return sysfsError(filepath.Join(base, entry), "%w", err)
	}
	*dst = v
	return nil
}

// readSysfsEPP reads a sysfs file and parses it as an EPP value.
func readSysfsEPP(base, entry string, dst *EPP) error {
	buf, err := readSysfsRaw(base, entry)
	if err != nil {
		return err
	}
	*dst = EPPFromString(buf)
	return nil
}

// writeSysfsRaw writes buf (plus a trailing newline) to the file at path.
func writeSysfsRaw(path, buf string) (err error) {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return sysfsError(path, "cannot open: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = sysfsError(path, "failed to close: %w", cerr)
		}
	}()
	if _, err = f.Write([]byte(buf + "\n")); err != nil {
		return sysfsError(path, "cannot write: %w", err)
	}
	return nil
}

// writeSysfsInt writes a signed integer to a sysfs file. When old is provided,
// the previous value is read into *old before writing.
func writeSysfsInt[T signedInt](base, entry string, val T, old ...*T) error {
	if len(old) > 0 && old[0] != nil {
		if err := readSysfsInt(base, entry, old[0]); err != nil {
			return err
		}
	}
	return writeSysfsRaw(filepath.Join(base, entry), fmt.Sprintf("%d", val))
}

// writeSysfsUint writes an unsigned integer to a sysfs file. When old is
// provided, the previous value is read into *old before writing.
func writeSysfsUint[T unsignedInt](base, entry string, val T, old ...*T) error {
	if len(old) > 0 && old[0] != nil {
		if err := readSysfsUint(base, entry, old[0]); err != nil {
			return err
		}
	}
	return writeSysfsRaw(filepath.Join(base, entry), fmt.Sprintf("%d", val))
}

// parseIntTo parses str into the signed integer type T with the exact bit-width
// of T, so values outside T's range are rejected by strconv rather than truncated.
// Named types whose underlying type is int (e.g. idset.ID) use strconv.IntSize.
func parseIntTo[T signedInt](str string) (T, error) {
	var zero T
	var bits int
	switch any(zero).(type) {
	case int8:
		bits = 8
	case int16:
		bits = 16
	case int32:
		bits = 32
	case int64:
		bits = 64
	default: // int and any named type whose underlying type is int
		bits = strconv.IntSize
	}
	v, err := strconv.ParseInt(str, 0, bits)
	return T(v), err
}

// parseUintTo parses str into the unsigned integer type T with the exact bit-width
// of T, so values outside T's range are rejected by strconv rather than truncated.
// Named types whose underlying type is uint (e.g. custom ID types) use strconv.IntSize.
func parseUintTo[T unsignedInt](str string) (T, error) {
	var zero T
	var bits int
	switch any(zero).(type) {
	case uint8:
		bits = 8
	case uint16:
		bits = 16
	case uint32:
		bits = 32
	case uint64:
		bits = 64
	default: // uint and any named type whose underlying type is uint
		bits = strconv.IntSize
	}
	v, err := strconv.ParseUint(str, 0, bits)
	return T(v), err
}

// parseIntSlice splits str by sep and parses each token into a []T, using the
// exact bit-width of T so out-of-range values are rejected rather than truncated.
func parseIntSlice[T signedInt](str, sep string) ([]T, error) {
	var out []T
	for _, s := range strings.Split(str, sep) {
		if s == "" {
			continue
		}
		v, err := parseIntTo[T](s)
		if err != nil {
			return nil, fmt.Errorf("invalid entry %q: %w", s, err)
		}
		out = append(out, v)
	}
	return out, nil
}

// parseUintSlice splits str by sep and parses each token into a []T, using the
// exact bit-width of T so out-of-range values are rejected rather than truncated.
func parseUintSlice[T unsignedInt](str, sep string) ([]T, error) {
	var out []T
	for _, s := range strings.Split(str, sep) {
		if s == "" {
			continue
		}
		v, err := parseUintTo[T](s)
		if err != nil {
			return nil, fmt.Errorf("invalid entry %q: %w", s, err)
		}
		out = append(out, v)
	}
	return out, nil
}

// parseIDSet parses a separator-delimited string of integers and ranges (e.g.
// "0-3 8 12-15") into an idset.IDSet.
func parseIDSet(str, sep string, dst *idset.IDSet) error {
	ids := idset.NewIDSet()
	for _, s := range strings.Split(str, sep) {
		if s == "" {
			continue
		}
		rng := strings.SplitN(s, "-", 2)
		if len(rng) == 1 {
			id, err := strconv.Atoi(s)
			if err != nil {
				return fmt.Errorf("invalid entry %q: %w", s, err)
			}
			ids.Add(idset.ID(id))
		} else {
			beg, err := strconv.Atoi(rng[0])
			if err != nil {
				return fmt.Errorf("invalid entry %q: %w", s, err)
			}
			end, err := strconv.Atoi(rng[1])
			if err != nil {
				return fmt.Errorf("invalid entry %q: %w", s, err)
			}
			for id := beg; id <= end; id++ {
				ids.Add(idset.ID(id))
			}
		}
	}
	*dst = ids
	return nil
}

// IDSetFromCPUSet returns an id set corresponding to a cpuset.CPUSet.
func IDSetFromCPUSet(cset cpuset.CPUSet) idset.IDSet {
	return idset.NewIDSetFromIntSlice(cset.List()...)
}

// CPUSetFromIDSet returns a cpuset.CPUSet corresponding to an id set.
func CPUSetFromIDSet(s idset.IDSet) cpuset.CPUSet {
	return cpuset.New(s.Members()...)
}

// GetMemoryCapacity parses memory capacity from /proc/meminfo (mimicking cAdvisor).
func GetMemoryCapacity() int64 {
	var (
		data []byte
		err  error
		capa int64
	)

	if data, err = os.ReadFile("/proc/meminfo"); err != nil {
		return -1
	}

	for _, line := range strings.Split(string(data), "\n") {
		keyval := strings.Split(line, ":")
		if len(keyval) != 2 || keyval[0] != "MemTotal" {
			continue
		}

		valunit := strings.Split(strings.TrimSpace(keyval[1]), " ")
		if len(valunit) != 2 || valunit[1] != "kB" {
			return -1
		}

		capa, err = strconv.ParseInt(valunit[0], 10, 64)
		if err != nil {
			return -1
		}

		capa *= 1024
		break
	}

	return capa
}
