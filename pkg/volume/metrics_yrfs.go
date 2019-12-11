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

package volume

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/volume/util/yrfs"
)

var _ MetricsProvider = &metricsYRFS{}

// metricsDu represents a MetricsProvider that calculates the used and
// available Volume space by calling fs.DiskUsage() and gathering
// filesystem info for the Volume path.
type metricsYRFS struct {
	// the directory path the volume is mounted to.
	pathTmp string
	path    string
}

// NewMetricsYRFS creates a new metricsYRFS with the Volume path.
// path: PersistentVolumePath
func NewMetricsYRFS(pathTmp string, path string) MetricsProvider {
	return &metricsYRFS{pathTmp,path}
}

// GetMetrics calculates the volume usage and device free space by executing
// "cat cat /proc/fs/yrfs/*/project_quota_info | grep path | awk '//{print $2 $3}'"
// and gathering filesystem info for the Volume path.
// See MetricsProvider.GetMetrics
func (md *metricsYRFS) GetMetrics() (*Metrics, error) {
	metrics := &Metrics{Time: metav1.Now()}
	if md.path == "" {
		return metrics, NewNoPathDefinedError()
	}

	err := md.runDiskUsage(metrics)
	if err != nil {
		klog.Error("[Volume] Failed to get disk usage, error: ", err)
		return metrics, err
	}

	err = md.runFind(metrics)
	if err != nil {
		klog.Error("[Volume] Failed to get disk inodes usage, error: ", err)
		return metrics, err
	}

	err = md.getFsInfo(metrics)
	if err != nil {
		klog.Error("[Volume] Failed to get disk fsInfo, error: ", err)
		return metrics, err
	}

	return metrics, nil
}

// runDiskUsage gets disk usage of md.path and writes the results to metrics.Used
func (md *metricsYRFS) runDiskUsage(metrics *Metrics) error {
	used, err := yrfs.DiskUsage(md.pathTmp)
	if err != nil {
		return err
	}
	metrics.Used = used
	return nil
}

// runFind executes the "find" command and writes the results to metrics.InodesUsed
func (md *metricsYRFS) runFind(metrics *Metrics) error {
	inodesUsed, err := yrfs.Find(md.path)
	if err != nil {
		return err
	}
	metrics.InodesUsed = resource.NewQuantity(inodesUsed, resource.BinarySI)
	return nil
}

// getFsInfo writes metrics.Capacity and metrics.Available from the filesystem
// info
func (md *metricsYRFS) getFsInfo(metrics *Metrics) error {
	available, capacity, _, inodes, inodesFree, _, err := yrfs.FsInfo(md.path)
	if err != nil {
		return NewFsInfoFailedError(err)
	}
	metrics.Available = resource.NewQuantity(available, resource.BinarySI)
	metrics.Capacity = resource.NewQuantity(capacity, resource.BinarySI)
	metrics.Inodes = resource.NewQuantity(inodes, resource.BinarySI)
	metrics.InodesFree = resource.NewQuantity(inodesFree, resource.BinarySI)
	return nil
}
