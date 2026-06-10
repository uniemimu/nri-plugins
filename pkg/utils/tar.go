// Copyright 2019-2021 Intel Corporation. All Rights Reserved.
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

package utils

import (
	"archive/tar"
	"compress/bzip2"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func UncompressTbz2(archive string, dir string) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close() // nolint:errcheck

	rootAbs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}

	data := bzip2.NewReader(file)
	tr := tar.NewReader(data)
	for {
		header, err := tr.Next()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		cleanName := filepath.Clean(header.Name)
		if filepath.IsAbs(cleanName) || cleanName == ".." || strings.HasPrefix(cleanName, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("invalid archive entry path: %s", header.Name)
		}

		targetPath := filepath.Join(dir, cleanName)
		targetAbs, err := filepath.Abs(targetPath)
		if err != nil {
			return err
		}
		if targetAbs != rootAbs && !strings.HasPrefix(targetAbs, rootAbs+string(os.PathSeparator)) {
			return fmt.Errorf("invalid archive entry path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			// Create a directory.
			err = os.MkdirAll(targetAbs, 0755)
			if err != nil {
				return err
			}
		case tar.TypeReg:
			// Create a regular file.
			err = os.MkdirAll(filepath.Dir(targetAbs), 0755)
			if err != nil {
				return err
			}
			targetFile, err := os.Create(targetAbs)
			if err != nil {
				return err
			}
			if _, err := io.Copy(targetFile, tr); err != nil {
				return err
			}
			if err := targetFile.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			// Create a symlink and all the directories it needs.
			err = os.MkdirAll(filepath.Dir(targetAbs), 0755)
			if err != nil {
				return err
			}
			err := os.Symlink(header.Linkname, targetAbs)
			if err != nil {
				return err
			}
		}
	}
}
