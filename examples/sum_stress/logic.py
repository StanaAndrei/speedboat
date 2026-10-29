import json
import sys

def cpu_burner(cycles=10_000_000):
    """
    Continuously performs arithmetic operations to stress the processor.
    Batches the list comprehension up to 10M iterations per chunk to keep
    memory consumption bounded (preventing MemoryError) while pegging CPU at 100%.
    """
    # Continuously performs arithmetic operations
    batch_size = min(cycles, 10_000_000)
    remaining = cycles
    while remaining > 0:
        chunk = min(remaining, batch_size)
        _ = [x * x for x in range(chunk)]
        remaining -= chunk

def main():
    # 1. Read task JSON from stdin
    try:
        raw_input = sys.stdin.read()
        if not raw_input.strip():
            sys.stderr.write("Error: No task data received on stdin\n")
            sys.exit(1)
        task = json.loads(raw_input)
    except Exception as e:
        sys.stderr.write(f"Failed to parse task JSON from stdin: {e}\n")
        sys.exit(1)

    task_id = task.get("task_id", "")
    params = task.get("params", {})
    start = params.get("start", 0)
    end = params.get("end", 0)
    burn_cycles = params.get("burn_cycles", 10_000_000)

    # 2. Stress the CPU with arithmetic operations
    cpu_burner(burn_cycles)

    # 3. Compute interval sum using a for loop (not the closed-form formula)
    interval_sum = 0
    for i in range(start, end + 1):
        interval_sum += i

    # 4. Emit Result JSON to stdout
    result = {
        "task_id": task_id,
        "status": "ok",
        "output": {
            "start": start,
            "end": end,
            "sum": interval_sum
        }
    }
    json.dump(result, sys.stdout, indent=2)
    sys.stdout.write("\n")

if __name__ == "__main__":
    main()
