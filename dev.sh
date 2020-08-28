#!/bin/bash -e
cd web
yarn
yarn build
cd ..
go run *.go