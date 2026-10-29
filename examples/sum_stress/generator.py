import json
import sys

def main():
    total_range = 50_000_000
    chunk_size = 5_000_000
    burn_cycles_per_task = 10_000_000  # Stresses the processor per task
    tasks = []

    task_index = 0
    for start in range(1, total_range + 1, chunk_size):
        end = min(start + chunk_size - 1, total_range)
        tasks.append({
            "task_id": f"task-sum-{task_index:04d}",
            "params": {
                "start": start,
                "end": end,
                "burn_cycles": burn_cycles_per_task
            }
        })
        task_index += 1

    # Emit JSON array to stdout for the Master Node to ingest
    json.dump(tasks, sys.stdout, indent=2)

if __name__ == "__main__":
    main()
