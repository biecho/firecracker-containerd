// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"

	"github.com/firecracker-microvm/firecracker-containerd/config"
	"github.com/firecracker-microvm/firecracker-containerd/internal/vm"
	"github.com/firecracker-microvm/firecracker-containerd/proto"
	firecracker "github.com/firecracker-microvm/firecracker-go-sdk"
)

const (
	kernelImageFileName   = "kernel-image"
	jailerHandlerName     = "firecracker-containerd-jail-handler"
	jailerFifoHandlerName = "firecracker-containerd-jail-fifo-handler"
	rootfsFolder          = "rootfs"
)

// jailer will allow modification and provide options to the the Firecracker VM
// to allow for jailing. In addition, this will allow for given files to be exposed
// to the jailed filesystem.
type jailer interface {
	// BuildJailedMachine will modify the firecracker.Config and provide
	// firecracker.Opt to be passed into firecracker.NewMachine which will allow
	// for the VM to be jailed.
	BuildJailedMachine(cfg *config.Config, machineCfg *firecracker.Config, vmID string) ([]firecracker.Opt, error)
	// ExposeFileToJail will expose the given file to the jailed filesystem, including
	// regular files and block devices. An error is returned if provided a path to a file
	// with type that is not supported.
	ExposeFileToJail(path string) error
	// JailPath is used to return the directory we are supposed to be working in.
	JailPath() vm.Dir
	// StubDrivesOptions will return a set of options used to create a new stub
	// drive file
	StubDrivesOptions() []FileOpt

	// Stop the jailer as a way that is visible from the user-level process (e.g. SIGTERM).
	Stop(force bool) error

	// Close will do any necessary cleanup that the jailer has accrued.
	Close() error
}

type cgroupPather interface {
	CgroupPath() string
}

// FileOpt is a functional option that operates on an open file, modifying it to be usable
// by the jailer implementation providing the option.
type FileOpt func(*os.File) error

// newJailer is used to construct a jailer from the CreateVM request. If no
// request or jailer config was provided, then the noopJailer will be returned.
func newJailer(
	ctx context.Context,
	logger *logrus.Entry,
	ociBundlePath string,
	service *service,
	request *proto.CreateVMRequest,
) (jailer, error) {
	fmt.Println("1")
	if request == nil || request.JailerConfig == nil {
		l := logger.WithField("jailer", "noop")
		return newNoopJailer(ctx, l, service.shimDir), nil
	}

	fmt.Println("2")
	if request.JailerConfig.UID == 0 || request.JailerConfig.GID == 0 {
		return nil, fmt.Errorf(
			"attempting to run as %d:%d. 0 cannot be used for the UID or GID",
			request.JailerConfig.UID,
			request.JailerConfig.GID,
		)
	}

	fmt.Println("3")
	if err := os.MkdirAll(ociBundlePath, 0700); err != nil {
		return nil, fmt.Errorf("failed to create oci bundle path: %s: %w", ociBundlePath, err)
	}

	fmt.Println("4")
	l := logger.WithField("jailer", "runc")
	config := runcJailerConfig{
		OCIBundlePath:     ociBundlePath,
		RuncBinPath:       service.config.JailerConfig.RuncBinaryPath,
		RuncConfigPath:    service.config.JailerConfig.RuncConfigPath,
		UID:               request.JailerConfig.UID,
		GID:               request.JailerConfig.GID,
		CPUs:              request.JailerConfig.CPUs,
		Mems:              request.JailerConfig.Mems,
		CgroupPath:        request.JailerConfig.CgroupPath,
		DriveExposePolicy: request.JailerConfig.DriveExposePolicy,
	}
	return newRuncJailer(ctx, l, service.vmID, config, request.DriveMounts)
}
