#!/bin/sh
# one section at a time; each section removes what it made before the next starts
cd "$(dirname "$0")" || exit 1
export RSS_LIMIT_GB=24 BENCH_RESULTS=results-9e292de.jsonl
ALL="graphify redis curl opencv typescript k8s vscode"
L=queue-9e292.log
while kill -0 96857 2>/dev/null; do sleep 10; done      # stage go, already running
rm -rf out; echo "STAGE go DONE" >> $L
BENCH_TOOLS=old,new,new-nc python3 bench.py 3 $ALL >> $L 2>&1   # no-op unless stage go stopped early
BENCH_TOOLS=up-nc python3 bench.py 3 $ALL >> $L 2>&1 && echo "STAGE upnc DONE" >> $L || echo "STAGE upnc FAILED" >> $L
rm -rf out
/opt/homebrew/opt/python@3.14/bin/python3.14 -m venv venv-leiden && nice -n 19 ./venv-leiden/bin/pip install -q --no-cache-dir 'graphifyy[leiden]==0.9.88' >> $L 2>&1
./venv-leiden/bin/pip freeze > runs/venv-leiden.freeze.txt 2>/dev/null; ./venv/bin/pip freeze > runs/venv.freeze.txt 2>/dev/null
BENCH_TOOLS=up-ld BENCH_PHASES=cold python3 bench.py 3 redis opencv k8s >> $L 2>&1 && echo "STAGE upld DONE" >> $L || echo "STAGE upld FAILED" >> $L
rm -rf out venv-leiden
echo QUEUE-END >> $L
