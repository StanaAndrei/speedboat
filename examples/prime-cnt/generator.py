import json
import sys

def main():
    total_range = 10_000_000
    chunk_size = 500_000
    tasks = []

    task_index = 0
    for start in range(2, total_range, chunk_size):
        end = min(start + chunk_size - 1, total_range)
        tasks.append({
            "task_id": f"task-prime-{task_index:04d}",
            "params": {
                "start": start,
                "end": end
            }
        })
        task_index += 1

    # Emit JSON array to stdout for the Master Node to ingest
    json.dump(tasks, sys.stdout, indent=2)

if __name__ == "__main__":
    main()