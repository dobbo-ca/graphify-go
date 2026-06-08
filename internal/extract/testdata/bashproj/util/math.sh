#!/usr/bin/env bash

add() {
	local sum=$(( $1 + $2 ))
	echo "$sum"
}

double() {
	add "$1" "$1"
}
