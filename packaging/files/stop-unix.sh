#!/bin/sh
# Stops Agentry.
cd "$(dirname "$0")" || exit 1
./agentry stop
