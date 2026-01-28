import {check} from 'k6';
import http from 'k6/http';
import {WebSocket} from 'k6/experimental/websockets';
import crypto from 'k6/crypto';
import {uuidv4} from 'https://jslib.k6.io/k6-utils/1.6.0/index.js';
import {expect} from 'https://jslib.k6.io/k6-testing/0.6.1/index.js';

const BASE_URL = __ENV.ORCHARD_BASE_URL || 'http://127.0.0.1:6120/v1';
const WS_BASE_URL = __ENV.ORCHARD_WS_BASE_URL || BASE_URL.replace(/^http/, 'ws');
const WS_BYTES = __ENV.WS_BYTES || 64 * 1024;

export const options = {
    scenarios: {
        vmLifecycle: {
            executor: 'ramping-vus',
            stages: [
                {duration: '5m', target: 2_000},
            ],
        },
    },
};

export default async function () {
    const vmName = `k6-${uuidv4()}`;

    createVM(vmName);

    await portForward(vmName);

    deleteVM(vmName);
}

function createVM(vmName) {
    const loremIpsum = `Lorem ipsum
    dolor sit amet,
    consectetur adipiscing elit.
    Fusce at orci nisi.
    Donec lacinia neque et risus elementum,
    ut interdum lacus pretium.
`;

    const body = JSON.stringify({
        name: vmName,
        image: 'ghcr.io/cirruslabs/macos-tahoe-base:latest',
        headless: true,
        startup_script: {
            script_content: loremIpsum
        },
    });

    const resp = http.post(`${BASE_URL}/vms`, body, {
        headers: {
            'Content-Type': 'application/json',
        },
    });

    const ok = check(resp, {
        'VM creation succeeded': (r) => r.status === 200,
    });

    if (!ok) {
        console.error(`Failed to create a VM: HTTP ${resp.status}`);
    }
}

async function portForward(vmName) {
    const url = `${WS_BASE_URL}/vms/${vmName}/port-forward?port=22&wait=60`;

    console.debug(`connecting to ${url}`);

    const ws = new WebSocket(url);
    ws.binaryType = 'arraybuffer';

    const sentBytes = new Uint8Array(crypto.randomBytes(WS_BYTES));
    let receivedBytes = new Uint8Array();

    const evt = await new Promise((resolve, reject) => {
        ws.onopen = () => {
            ws.send(sentBytes);
        };

        ws.onmessage = (event) => {
            if (event.data instanceof ArrayBuffer) {
                const chunk = new Uint8Array(event.data);

                const combined = new Uint8Array(receivedBytes.length + chunk.length);
                combined.set(receivedBytes);
                combined.set(chunk, receivedBytes.length);
                receivedBytes = combined;
            }

            if (receivedBytes.length >= WS_BYTES) {
                ws.close();
            }
        };

        ws.onerror = (evt) => {
            reject(new Error(`WebSocket error: ${evt.error}`));
        };

        ws.onclose = (evt) => {
            resolve(evt);
        };
    });

    expect(evt.code).toBe(1000);
    expect(receivedBytes).toEqual(sentBytes);
}

function deleteVM(vmName) {
    const resp = http.del(`${BASE_URL}/vms/${vmName}`);

    const ok = check(resp, {
        'VM deletion succeeded': (r) => r.status === 200,
    });

    if (!ok) {
        console.error(`Failed to delete a VM: HTTP ${resp.status}`);
    }
}
