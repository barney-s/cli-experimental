// Code generated by Wire. DO NOT EDIT.

//go:generate wire
//+build !wireinject

package pkg

import (
	"io"
	"sigs.k8s.io/cli-experimental/internal/pkg/apply"
	delete2 "sigs.k8s.io/cli-experimental/internal/pkg/delete"
	"sigs.k8s.io/cli-experimental/internal/pkg/prune"
	"sigs.k8s.io/cli-experimental/internal/pkg/util"
	"sigs.k8s.io/cli-experimental/internal/pkg/wirecli/wirek8s"
)

// Injectors from wire.go:

func InitializeCmd(writer io.Writer, args util.Args) (*Cmd, error) {
	configFlags, err := wirek8s.NewConfigFlags(args)
	if err != nil {
		return nil, err
	}
	config, err := wirek8s.NewRestConfig(configFlags)
	if err != nil {
		return nil, err
	}
	dynamicInterface, err := wirek8s.NewDynamicClient(config)
	if err != nil {
		return nil, err
	}
	restMapper, err := wirek8s.NewRestMapper(config)
	if err != nil {
		return nil, err
	}
	client, err := wirek8s.NewClient(dynamicInterface, restMapper)
	if err != nil {
		return nil, err
	}
	applyApply := &apply.Apply{
		DynamicClient: client,
		Out:           writer,
	}
	prunePrune := &prune.Prune{
		DynamicClient: client,
		Out:           writer,
	}
	deleteDelete := &delete2.Delete{
		DynamicClient: client,
		Out:           writer,
	}
	cmd := &Cmd{
		Applier: applyApply,
		Pruner:  prunePrune,
		Deleter: deleteDelete,
	}
	return cmd, nil
}
