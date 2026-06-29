#!/bin/bash

mkdir -p dist
go build -o dist/cfn main.go
mkdir -p $HOME/local/bin

mv dist/cfn $HOME/local/bin/cfn
