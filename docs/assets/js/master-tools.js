/**
 * Project Speedboat: Master Node Interactive Config & Contract Playground
 * Specification: arch_docs/project.txt (Scientist UX: Math-to-Grid Pipeline)
 */

document.addEventListener('DOMContentLoaded', () => {
  initMasterConfigPlayground();
});

function initMasterConfigPlayground() {
  const langSelect = document.getElementById('cfg-lang');
  const nameInput = document.getElementById('cfg-name');
  const targetSelect = document.getElementById('cfg-target');
  const redundancySelect = document.getElementById('cfg-redundancy');

  const yamlOutput = document.getElementById('code-project-yaml');
  const logicOutput = document.getElementById('code-logic-file');
  const logicFileTitle = document.getElementById('logic-file-title');

  if (!langSelect || !nameInput || !yamlOutput) return;

  const templates = {
    cpp: {
      ext: 'logic.cpp',
      code: `// logic.cpp - Speedboat Scientific Worker
#include <iostream>
#include <string>
#include <cmath>

bool is_prime(long long n) {
    if (n <= 1) return false;
    if (n <= 3) return true;
    if (n % 2 == 0 || n % 3 == 0) return false;
    for (long long i = 5; i * i <= n; i += 6) {
        if (n % i == 0 || n % (i + 2) == 0) return false;
    }
    return true;
}

int main() {
    // Standard Stream Contract: Read task JSON from stdin
    std::string line;
    while (std::getline(std::cin, line)) {
        // Parse range: e.g. {"start": 1000, "end": 5000}
        // Scientific computation runs here...
        long long primes_found = 42; 
        
        // Emit calculated result JSON to stdout
        std::cout << "{\\"status\\":\\"success\\",\\"primes\\":" << primes_found << "}\\n";
    }
    return 0;
}`
    },
    rust: {
      ext: 'logic.rs',
      code: `// logic.rs - Speedboat Scientific Worker
use std::io::{self, BufRead};

fn is_prime(n: u64) -> bool {
    if n <= 1 { return false; }
    if n <= 3 { return true; }
    if n % 2 == 0 || n % 3 == 0 { return false; }
    let mut i = 5;
    while i * i <= n {
        if n % i == 0 || n % (i + 2) == 0 { return false; }
        i += 6;
    }
    true
}

fn main() {
    let stdin = io::stdin();
    for line in stdin.lock().lines() {
        if let Ok(_task_json) = line {
            // Perform computation
            let count = 42;
            println!("{{\\"status\\":\\"success\\",\\"primes\\":{}}}", count);
        }
    }
}`
    },
    python: {
      ext: 'logic.py',
      code: `# logic.py - Speedboat Scientific Worker
import sys
import json

def is_prime(n):
    if n <= 1: return False
    if n <= 3: return True
    if n % 2 == 0 or n % 3 == 0: return False
    i = 5
    while i * i <= n:
        if n % i == 0 or n % (i + 2) == 0: return False
        i += 6
    return True

for line in sys.stdin:
    if not line.strip(): continue
    task = json.loads(line)
    start = task.get("start", 1000)
    end = task.get("end", 5000)
    
    count = sum(1 for x in range(start, end) if is_prime(x))
    # Emit result JSON to stdout
    print(json.dumps({"status": "success", "primes": count}))
    sys.stdout.flush()
`
    },
    fortran: {
      ext: 'logic.f90',
      code: `! logic.f90 - Speedboat High-Performance Science Worker
program prime_worker
  implicit none
  character(len=1024) :: input_buffer
  integer :: primes_found = 42

  do
    read(*, '(A)', end=100) input_buffer
    ! High-performance calculation runs here
    write(*, '("{\""status\"":\""success\"",\""primes\"":", I0, "}")') primes_found
  end do

100 continue
end program prime_worker`
    }
  };

  function update() {
    const lang = langSelect.value;
    const name = nameInput.value.trim() || 'prime-counter';
    const target = targetSelect.value;
    const redundancy = parseInt(redundancySelect.value, 10) || 3;
    const consensus = Math.ceil((redundancy * 2) / 3);

    let containerYaml = '';
    if (target === 'docker') {
      containerYaml = `container:
  dockerfile: "./Dockerfile"
  image_tag: "${name}:latest"`;
    } else {
      containerYaml = `wasm:
  build_target: "wasi"
  source_file: "./${templates[lang].ext}"
  output_wasm: "./logic.wasm"`;
    }

    const yaml = `name: ${name}
version: "1.0.0"
${containerYaml}
generator:
  runtime: python3
  source: generator.py
reducer:
  runtime: python3
  source: reducer.py
verification:
  redundancy: ${redundancy}
  consensus_threshold: ${consensus}
  floating_point_tolerance: 1e-7
  iterations: 10`;

    yamlOutput.innerText = yaml;

    if (logicOutput && templates[lang]) {
      logicOutput.innerText = templates[lang].code;
      if (logicFileTitle) logicFileTitle.innerText = templates[lang].ext;
    }
  }

  langSelect.addEventListener('change', update);
  nameInput.addEventListener('input', update);
  targetSelect.addEventListener('change', update);
  redundancySelect.addEventListener('change', update);

  update();
}
