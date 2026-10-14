#!/bin/sh
# graphify-go-dma: upstream Louvain, Leiden and --no-cluster, interleaved, one session
cd "$(dirname "$0")" || exit 1
export RSS_LIMIT_GB=24 BENCH_RESULTS=results-dma.jsonl BENCH_PHASES=cold BENCH_DISK_BUDGET_GB=32 BENCH_SWAP_BUDGET_GB=8
L=queue-dma.log
/opt/homebrew/opt/python@3.14/bin/python3.14 -m venv venv-leiden && nice -n 19 ./venv-leiden/bin/pip install -q --no-cache-dir 'graphifyy[leiden]==0.9.88' >> $L 2>&1
BENCH_TOOLS=up,up-ld,up-nc python3 bench.py 3 k8s >> $L 2>&1 && echo "STAGE dma DONE" >> $L || echo "STAGE dma FAILED" >> $L
rm -rf out venv-leiden
echo QUEUE-END >> $L
