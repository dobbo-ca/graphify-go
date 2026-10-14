#!/bin/sh
# check: run1.py stops a run that eats the disk budget, and the file is removed after
cd "$(dirname "$0")" || exit 1
d=/tmp/zz-guard-7c2e; rm -rf $d; mkdir -p $d
free=$(python3 -c "import shutil; print(shutil.disk_usage('/tmp').free/2**30)")
lim=$(python3 -c "print($free - 1)")
out=$(GRAPHIFY_OUT=$d DISK_MIN_FREE_GB=$lim python3 run1.py /tmp $d.log -- dd if=/dev/zero of=$d/f bs=1m count=3000)
rm -rf $d $d.log
echo "$out"
echo "$out" | grep -q '"killed": "disk_limit"' && echo "PASS disk guard" || { echo "FAIL disk guard"; exit 1; }
# an out dir that does not exist yet, as in a real run
out=$(GRAPHIFY_OUT=$d/not/yet python3 run1.py /tmp $d.log -- true)
rm -rf $d $d.log
echo "$out" | grep -q '"exit": 0' && echo "PASS missing out dir" || { echo "FAIL missing out dir"; exit 1; }
