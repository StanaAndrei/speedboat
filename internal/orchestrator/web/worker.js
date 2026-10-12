// Speedboat Web Worker - Volunteer WASM Task Processor
import { connect, JSONCodec } from "https://unpkg.com/nats.ws/esm/nats.js";

let config = null;
let isPaused = false;
let nc = null;
let jc = JSONCodec();
let wasmModule = null;

function postLog(msg, level = "info") {
  self.postMessage({ type: "log", data: { msg, level } });
}

function postStatus(state, text) {
  self.postMessage({ type: "status", data: { state, text } });
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

// Minimal, zero-dependency WASI Preview-1 runner
async function executeWASI(compiledModule, inputStr) {
  const enc = new TextEncoder();
  const inputBytes = enc.encode(inputStr);
  let inputOffset = 0;
  const stdoutChunks = [];
  let memory = null;

  return new Promise(async (resolve, reject) => {
    const wasi = {
      environ_sizes_get: (countPtr, sizePtr) => {
        const view = new DataView(memory.buffer);
        view.setUint32(countPtr, 0, true);
        view.setUint32(sizePtr, 0, true);
        return 0;
      },
      environ_get: () => 0,
      clock_time_get: (id, precision, timePtr) => {
        const view = new DataView(memory.buffer);
        const now = BigInt(Date.now()) * 1000000n;
        view.setBigUint64(timePtr, now, true);
        return 0;
      },
      fd_close: () => 0,
      fd_fdstat_get: (fd, statPtr) => {
        const view = new DataView(memory.buffer);
        view.setUint8(statPtr, 2); // character device
        view.setUint16(statPtr + 2, 0, true);
        return 0;
      },
      fd_prestat_get: () => 8, // BADF
      fd_prestat_dir_name: () => 8,
      fd_read: (fd, iovs, iovsLen, nreadPtr) => {
        const view = new DataView(memory.buffer);
        let bytesRead = 0;
        for (let i = 0; i < iovsLen; i++) {
          const ptr = view.getUint32(iovs + i * 8, true);
          const len = view.getUint32(iovs + i * 8 + 4, true);
          const toCopy = Math.min(len, inputBytes.length - inputOffset);
          if (toCopy > 0) {
            new Uint8Array(memory.buffer, ptr, toCopy).set(
              inputBytes.subarray(inputOffset, inputOffset + toCopy)
            );
            inputOffset += toCopy;
            bytesRead += toCopy;
          }
        }
        view.setUint32(nreadPtr, bytesRead, true);
        return 0;
      },
      fd_seek: () => 0,
      fd_write: (fd, iovs, iovsLen, nwrittenPtr) => {
        const view = new DataView(memory.buffer);
        let bytesWritten = 0;
        for (let i = 0; i < iovsLen; i++) {
          const ptr = view.getUint32(iovs + i * 8, true);
          const len = view.getUint32(iovs + i * 8 + 4, true);
          const chunk = new Uint8Array(memory.buffer.slice(ptr, ptr + len));
          if (fd === 1 || fd === 2) {
            stdoutChunks.push(chunk);
          }
          bytesWritten += len;
        }
        view.setUint32(nwrittenPtr, bytesWritten, true);
        return 0;
      },
      proc_exit: (code) => {
        const dec = new TextDecoder();
        const totalLen = stdoutChunks.reduce((acc, c) => acc + c.length, 0);
        const combined = new Uint8Array(totalLen);
        let offset = 0;
        for (const c of stdoutChunks) {
          combined.set(c, offset);
          offset += c.length;
        }
        resolve({ code, stdout: dec.decode(combined) });
      },
    };

    try {
      const instance = await WebAssembly.instantiate(compiledModule, {
        wasi_snapshot_preview1: wasi,
      });
      memory = instance.exports.memory;
      instance.exports._start();

      // If _start returned without proc_exit
      const dec = new TextDecoder();
      const totalLen = stdoutChunks.reduce((acc, c) => acc + c.length, 0);
      const combined = new Uint8Array(totalLen);
      let offset = 0;
      for (const c of stdoutChunks) {
        combined.set(c, offset);
        offset += c.length;
      }
      resolve({ code: 0, stdout: dec.decode(combined) });
    } catch (err) {
      // In WASI, proc_exit may throw a WebAssembly trap / RuntimeError to unwind
      const dec = new TextDecoder();
      const totalLen = stdoutChunks.reduce((acc, c) => acc + c.length, 0);
      const combined = new Uint8Array(totalLen);
      let offset = 0;
      for (const c of stdoutChunks) {
        combined.set(c, offset);
        offset += c.length;
      }
      const out = dec.decode(combined);
      if (out.trim().length > 0) {
        resolve({ code: 0, stdout: out });
      } else {
        reject(err);
      }
    }
  });
}

async function startWorker() {
  const { projectId, natsWs, workerId, wasmUrl } = config;

  try {
    // 1. Fetch and compile WASM payload
    postLog(`📦 Fetching WASM payload from ${wasmUrl}...`, "info");
    const wasmResp = await fetch(wasmUrl);
    if (!wasmResp.ok) {
      throw new Error(`Failed to fetch WASM: HTTP ${wasmResp.status}`);
    }
    const wasmBuf = await wasmResp.arrayBuffer();
    const sizeMb = (wasmBuf.byteLength / (1024 * 1024)).toFixed(2);
    postLog(`📦 Compiling WASM module (${sizeMb} MB)...`, "info");
    wasmModule = await WebAssembly.compile(wasmBuf);
    postLog(`✓ WASM module ready for execution`, "success");

    // 2. Connect to NATS
    postLog(`🌐 Connecting to NATS WebSocket at ${natsWs}...`, "info");
    nc = await connect({ servers: natsWs });
    postLog(`✓ Connected to NATS cluster`, "success");
    postStatus("idle", "Idle / Waiting for Tasks");

    // 3. Task request and execution loop
    let lastCompletedTaskId = "";

    while (true) {
      while (isPaused) {
        await sleep(200);
      }

      const reqPayload = {
        project_id: projectId,
        worker_id: workerId,
        ip: "web-worker",
        completed_task_id: lastCompletedTaskId,
      };

      let respMsg = null;
      try {
        respMsg = await nc.request(
          `requests.${projectId}`,
          jc.encode(reqPayload),
          { timeout: 3000 }
        );
      } catch (err) {
        // Timeout or no responders, retry after delay
        await sleep(500);
        continue;
      }

      let resp = null;
      try {
        resp = jc.decode(respMsg.data);
      } catch (err) {
        await sleep(500);
        continue;
      }

      if (resp.status === "completed") {
        postLog("🏁 Project completed. All tasks finished!", "success");
        postStatus("completed", "Project Completed");
        break;
      }

      if (resp.status === "blacklisted") {
        postLog("🚫 Worker has been blacklisted by master.", "err");
        postStatus("paused", "Blacklisted");
        break;
      }

      if (resp.status === "no_tasks") {
        postStatus("idle", "Waiting for Available Tasks...");
        await sleep(1000);
        continue;
      }

      if (resp.status === "ok" && resp.task) {
        const taskObj = resp.task;
        const taskId = taskObj.task_id || "unknown";

        postStatus("computing", `Crunching ${taskId}...`);
        postLog(`📥 [Worker ${workerId}] Fetched task ${taskId}`, "info");

        const startTime = performance.now();
        const taskJsonStr = JSON.stringify(taskObj);

        let stdoutStr = "";
        let execErr = null;
        try {
          const res = await executeWASI(wasmModule, taskJsonStr);
          stdoutStr = res.stdout;
        } catch (err) {
          execErr = err;
        }

        const durationMs = Math.round(performance.now() - startTime);

        let taskResult = null;
        if (execErr) {
          postLog(`❌ [Worker ${workerId}] Task ${taskId} failed: ${execErr.message}`, "err");
          taskResult = {
            task_id: taskId,
            project_id: projectId,
            status: "error",
            error: execErr.message,
            worker_id: workerId,
            ip: "web-worker",
          };
        } else {
          // Attempt parsing stdout as TaskResult or wrap output
          try {
            taskResult = JSON.parse(stdoutStr);
            if (!taskResult.task_id) taskResult.task_id = taskId;
            taskResult.project_id = projectId;
            taskResult.worker_id = workerId;
            taskResult.ip = "web-worker";
            if (!taskResult.status) taskResult.status = "ok";
          } catch (e) {
            taskResult = {
              task_id: taskId,
              project_id: projectId,
              status: "ok",
              output: stdoutStr,
              worker_id: workerId,
              ip: "web-worker",
            };
          }

          postLog(`📤 [Worker ${workerId}] Task ${taskId} completed in ${durationMs}ms. Publishing result...`, "success");
        }

        // Publish result to results.<project_id>
        nc.publish(`results.${projectId}`, jc.encode(taskResult));
        await nc.flush();

        lastCompletedTaskId = taskId;
        self.postMessage({
          type: "task_completed",
          data: { taskId, durationMs },
        });

        postStatus("idle", "Idle / Waiting for Tasks");
      }
    }
  } catch (err) {
    postLog(`❌ Fatal worker error: ${err.message}`, "err");
    postStatus("paused", "Connection Error");
  }
}

self.onmessage = (e) => {
  const { action, config: newConfig } = e.data;
  if (action === "init") {
    config = newConfig;
    startWorker();
  } else if (action === "pause") {
    isPaused = true;
  } else if (action === "resume") {
    isPaused = false;
  }
};
