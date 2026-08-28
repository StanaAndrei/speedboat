import json
import sys

def main():
    try:
        # Master pipes verified batches via stdin
        batch = json.load(sys.stdin)
    except Exception as e:
        sys.stderr.write(f"Failed to parse stdin: {e}\n")
        sys.exit(1)

    results = batch.get("results", [])
    total_primes = 0
    intervals_processed = 0

    for res in results:
        if res.get("status") == "ok":
            output = res.get("output", {})
            total_primes += output.get("count", 0)
            intervals_processed += 1

    summary = {
        "project_id": batch.get("project_id", "prime-counter"),
        "intervals_processed": intervals_processed,
        "total_primes_found": total_primes
    }

    # Emit final reduced dataset
    json.dump(summary, sys.stdout, indent=2)
    sys.stdout.write("\n")

if __name__ == "__main__":
    main()