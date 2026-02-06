#!/bin/bash

mkdir -p dist
go build -o dist/cfn-list main.go
mkdir -p $HOME/local/bin
mv dist/cfn-list $HOME/local/bin/cfn-list
