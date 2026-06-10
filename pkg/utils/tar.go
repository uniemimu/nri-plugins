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

func safeJoin(rootAbs, name string) (string, error) {
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("absolute archive path not allowed: %s", name)
	}

	candidate := filepath.Join(rootAbs, name)
	parent := filepath.Dir(candidate)

	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		// Parent may not exist yet; fall back to absolute cleaned parent.
		resolvedParent, err = filepath.Abs(parent)
		if err != nil {
			return "", err
		}
	}

	finalPath := filepath.Join(resolvedParent, filepath.Base(candidate))
	finalAbs, err := filepath.Abs(finalPath)
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(rootAbs, finalAbs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("archive path escapes target dir: %s", name)
	}
	return finalAbs, nil
}

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
		switch header.Typeflag {
		case tar.TypeDir:
			targetPath, err := safeJoin(rootAbs, header.Name)
			if err != nil {
				return err
			}
			// Create a directory.
			err = os.MkdirAll(targetPath, 0755)
			if err != nil {
				return err
			}
		case tar.TypeReg:
			targetPath, err := safeJoin(rootAbs, header.Name)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return err
			}
			// Create a regular file.
			targetFile, err := os.Create(targetPath)
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
			linkPath, err := safeJoin(rootAbs, header.Name)
			if err != nil {
				return err
			}
			// Create a symlink and all the directories it needs.
			err = os.MkdirAll(filepath.Dir(linkPath), 0755)
			if err != nil {
				return err
			}
			err = os.Symlink(header.Linkname, linkPath)
			if err != nil {
				return err
			}
		}
	}
}
