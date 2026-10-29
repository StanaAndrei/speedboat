# `sum_stress`: Distributed Interval Summation & CPU Stress Test

`sum_stress` is an example workload for **Project Speedboat** demonstrating:
1. **Interval-based Problem Partitioning**: Decomposing a large summation problem ($\sum_{i=1}^{N} i$) across workers.
2. **Pure Python Worker Logic**: Demonstrating Python-based container execution instead of compiled languages (C++/Rust).
3. **CPU Stress & Burn Testing**: Performing intensive arithmetic operations in each task to stress the worker CPU cores.
4. **Analytical Consensus & Reduction**: Computing interval sums iteratively with `for` loops, then validating the aggregated total against the closed-form Gaussian sum formula $\frac{N(N+1)}{2}$.

---

## Architecture & Stream Contract

```
┌─────────────────┐       tasks.json       ┌───────────────────────┐
│  generator.py   │ ─────────────────────> │ Speedboat Master Node │
└─────────────────┘                        └──────────┬────────────┘
                                                      │ Task JSON via stdin
                                                      ▼
                                           ┌───────────────────────┐
                                           │  Docker / Container   │
                                           │   python3 logic.py    │
                                           │  - cpu_burner()       │
                                           │  - for loop sum       │
                                           └──────────┬────────────┘
                                                      │ Result JSON via stdout
                                                      ▼
┌─────────────────┐   ReducerBatch stdin   ┌───────────────────────┐
│   reducer.py    │ <───────────────────── │ Master Consensus Ver. │
└─────────────────┘                        └───────────────────────┘
```

### 1. Task Generator (`generator.py`)
Splits the total calculation range ($1 \dots 50,000,000$) into chunks of $5,000,000$. Each task specification includes:
```json
{
  "task_id": "task-sum-0000",
  "params": {
    "start": 1,
    "end": 5000000,
    "burn_cycles": 10000000
  }
}
```

### 2. Scientific Logic (`logic.py`)
- Reads task parameters from `stdin`.
- Executes `cpu_burner(burn_cycles)`: Performs arithmetic operations (`x * x`) in batched list comprehensions to stress the CPU at 100% load while remaining within safe memory bounds.
- Computes the interval sum using an iterative `for` loop (avoiding the closed-form formula $n(n+1)/2$).
- Returns the computed partial sum to `stdout`.

### 3. Reducer (`reducer.py`)
- Aggregates verified partial sums from all intervals.
- Verifies that $\sum \text{parts} == \frac{N(N+1)}{2}$.
- Outputs final summary report in JSON format:
```json
{
  "project_id": "sum_stress",
  "intervals_processed": 10,
  "range_start": 1,
  "range_end": 50000000,
  "total_sum": 1250000025000000,
  "expected_sum": 1250000025000000,
  "verified": true
}
```

---

## Testing Locally

### 1. Test Generator
```bash
python3 examples/sum_stress/generator.py | head -n 25
```

### 2. Test Logic Script Directly
```bash
echo '{"task_id":"test-0","params":{"start":1,"end":1000,"burn_cycles":100000}}' | python3 examples/sum_stress/logic.py
```

### 3. Test Reducer Directly
```bash
echo '{
  "project_id": "sum_stress",
  "results": [
    {"status":"ok","output":{"start":1,"end":500,"sum":125250}},
    {"status":"ok","output":{"start":501,"end":1000,"sum":375250}}
  ]
}' | python3 examples/sum_stress/reducer.py
```

### 4. Build and Test Container
```bash
docker build -t sum-stress:latest examples/sum_stress
echo '{"task_id":"test-0","params":{"start":1,"end":1000,"burn_cycles":100000}}' | docker run -i --rm sum-stress:latest
```
