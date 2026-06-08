#!/usr/bin/env bash

source "../util/math.sh"
. /etc/profile

boot() {
	local result
	result=$(add 2 3)
	echo "boot $result"
}

boot
