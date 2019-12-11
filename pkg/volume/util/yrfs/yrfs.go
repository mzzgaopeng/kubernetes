// +build linux darwin

/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package yrfs

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"

	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	cmdTmpl1 = "nice -n 19 cat /proc/fs/yrfs/*/project_quota_info | grep pvPath | awk '{print $2}'"
	cmdTmpl2 = "nice -n 19 cat /proc/fs/yrfs/*/project_quota_info | grep pvPath | awk '{print $3}'"
)

// FSInfo linux returns (available bytes, byte capacity, byte usage, total inodes, inodes free, inode usage, error)
// for the filesystem that path resides upon.
func FsInfo(path string) (int64, int64, int64, int64, int64, int64, error) {
	statfs := &unix.Statfs_t{}
	err := unix.Statfs(path, statfs)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, err
	}

	// Available is blocks available * fragment size
	available := int64(statfs.Bavail) * int64(statfs.Bsize)

	// Capacity is total block count * fragment size
	capacity := int64(statfs.Blocks) * int64(statfs.Bsize)

	// Usage is block being used * fragment size (aka block size).
	usage := (int64(statfs.Blocks) - int64(statfs.Bfree)) * int64(statfs.Bsize)

	inodes := int64(statfs.Files)
	inodesFree := int64(statfs.Ffree)
	inodesUsed := inodes - inodesFree

	return available, capacity, usage, inodes, inodesFree, inodesUsed, nil
}

// DiskUsage gets disk usage of specified path.
func DiskUsage(path string) (*resource.Quantity, error) {

	cmdline1 := strings.Replace(cmdTmpl1, "pvPath", path, -1)
	out, err := exec.Command("/bin/sh", "-c", cmdline1).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed command 'cat' ($ nice -n 19 cat /proc/fs/yrfs/*/project_quota_info | grep path ) on path %s, with error %v", path, err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("failed to get pvUsage value from /proc/fs/yrfs/*/project_quota_info, command: %s", cmdline1)
	}

	usedString := strings.Fields(string(out))[0]
	if usedString != "" && usedString != "0" {

		usedFloat64, err := strconv.ParseFloat(usedString, 64)
		if err != nil {
			return nil, fmt.Errorf("failed convert string to int, string: %s", usedString)
		}

		cmdline2 := strings.Replace(cmdTmpl2, "pvPath", path, -1)
		out, err := exec.Command("/bin/sh", "-c", cmdline2).CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("failed command 'cat' ($ nice -n 19 cat /proc/fs/yrfs/*/project_quota_info | grep path ) on path %s, with error %v", path, err)
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("failed to get pvUsage unit from /proc/fs/yrfs/*/project_quota_info, command: %s", cmdline1)
		}
		usedUnit := strings.Fields(string(out))[0]

		switch {
		case usedUnit == "KiB":
			usedFloat64 = usedFloat64 * 1024
		case usedUnit == "MiB":
			usedFloat64 = usedFloat64 * 1024 * 1024
		case usedUnit == "GiB":
			usedFloat64 = usedFloat64 * 1024 * 1024 * 1024
		case usedUnit == "TiB":
			usedFloat64 = usedFloat64 * 1024 * 1024 * 1024 * 1024
		default:
			usedFloat64 = usedFloat64 * 1
		}
		usedString = fmt.Sprintf("%.0f", usedFloat64)
	}

	used, err := resource.ParseQuantity(usedString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse 'pvUsed' output %s due to error %v", out, err)
	}

	used.Format = resource.BinarySI
	return &used, nil

}

// Find uses the equivalent of the command `find <path> -dev -printf '.' | wc -c` to count files and directories.
// While this is not an exact measure of inodes used, it is a very good approximation.
func Find(path string) (int64, error) {
	if path == "" {
		return 0, fmt.Errorf("invalid directory")
	}
	var counter byteCounter
	var stderr bytes.Buffer
	findCmd := exec.Command("find", path, "-xdev", "-printf", ".")
	findCmd.Stdout, findCmd.Stderr = &counter, &stderr
	if err := findCmd.Start(); err != nil {
		return 0, fmt.Errorf("failed to exec cmd %v - %v; stderr: %v", findCmd.Args, err, stderr.String())
	}
	if err := findCmd.Wait(); err != nil {
		return 0, fmt.Errorf("cmd %v failed. stderr: %s; err: %v", findCmd.Args, stderr.String(), err)
	}
	return counter.bytesWritten, nil
}

// Simple io.Writer implementation that counts how many bytes were written.
type byteCounter struct{ bytesWritten int64 }

func (b *byteCounter) Write(p []byte) (int, error) {
	b.bytesWritten += int64(len(p))
	return len(p), nil
}
