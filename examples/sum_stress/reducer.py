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
    total_sum = 0
    intervals_processed = 0
    min_start = None
    max_end = None

    for res in results:
        if res.get("status") == "ok":
            output = res.get("output", {})
            start = output.get("start")
            end = output.get("end")
            part_sum = output.get("sum", 0)

            total_sum += part_sum
            intervals_processed += 1

            if min_start is None or (start is not None and start < min_start):
                min_start = start
            if max_end is None or (end is not None and end > max_end):
                max_end = end

    # Analytical verification using Gauss formula
    expected_sum = 0
    if min_start is not None and max_end is not None:
        count = max_end - min_start + 1
        expected_sum = count * (min_start + max_end) // 2

    is_verified = (total_sum == expected_sum) and (intervals_processed > 0)

    summary = {
        "project_id": batch.get("project_id", "sum_stress"),
        "intervals_processed": intervals_processed,
        "range_start": min_start,
        "range_end": max_end,
        "total_sum": total_sum,
        "expected_sum": expected_sum,
        "verified": is_verified
    }

    # Emit final reduced dataset
    json.dump(summary, sys.stdout, indent=2)
    sys.stdout.write("\n")

if __name__ == "__main__":
    main()
