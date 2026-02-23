import {check} from 'k6';
import http from 'k6/http';
import {WebSocket} from 'k6/websockets';
import crypto from 'k6/crypto';
import encoding from 'k6/encoding';
import {uuidv4} from 'https://jslib.k6.io/k6-utils/1.6.0/index.js';

const BASE_URL = __ENV.BASE_URL || 'http://127.0.0.1:6120/v1';
const WS_BASE_URL = __ENV.WS_BASE_URL || BASE_URL.replace(/^http/, 'ws');
const WS_BYTES = Number(__ENV.WS_BYTES) || 64 * 1024;
const SERVICE_ACCOUNT_NAME = __ENV.SERVICE_ACCOUNT_NAME;
const SERVICE_ACCOUNT_TOKEN = __ENV.SERVICE_ACCOUNT_TOKEN;

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
            ...authHeaders(),
        },
    });

    check(resp, {
        'VM creation succeeded': (r) => r.status === 200,
    });
}

async function portForward(vmName) {
    const url = `${WS_BASE_URL}/vms/${vmName}/port-forward?port=22&wait=60`;

    console.debug(`connecting to ${url}`);

    const ws = new WebSocket(url, [], {
        headers: {
            ...authHeaders(),
        }
    });
    ws.binaryType = 'arraybuffer';

    const sentBytes = crypto.randomBytes(WS_BYTES);
    const sentHash = crypto.sha256(sentBytes, 'hex');
    let numReceivedBytes = 0;
    const receivedHasher = crypto.createHash('sha256');

    const result = await new Promise((resolve, reject) => {
        ws.onopen = () => {
            ws.send(sentBytes);
        };

        ws.onmessage = (event) => {
            if (event.data instanceof ArrayBuffer) {
                numReceivedBytes += event.data.byteLength;
                receivedHasher.update(event.data);
            }

            if (numReceivedBytes >= WS_BYTES) {
                ws.close();
            }
        };

        ws.onerror = (evt) => {
            resolve({
                error: evt.error,
            });
        };

        ws.onclose = (evt) => {
            resolve({
                code: evt.code,
            });
        };
    });

    check(result, {
        'WebSocket closed normally (1000)': (r) => r.code === 1000,
        'WebSocket had no errors': (r) => !r.error,
    });

    check(numReceivedBytes, {
        'WebSocket received exactly WS_BYTES back': (r) => WS_BYTES === r,
    });

    const receivedHash = receivedHasher.digest('hex');
    check(receivedHash, {
        'WebSocket received exactly the same data back': (r) => sentHash === receivedHash,
    });
}

function deleteVM(vmName) {
    const resp = http.del(`${BASE_URL}/vms/${vmName}`, null, {
        headers: {
            'Content-Type': 'application/json',
            ...authHeaders(),
        }
    });

    check(resp, {
        'VM deletion succeeded': (r) => r.status === 200,
    });
}

function authHeaders() {
    if (!SERVICE_ACCOUNT_NAME || !SERVICE_ACCOUNT_TOKEN) {
        return {}
    }

    const credentials = `${SERVICE_ACCOUNT_NAME}:${SERVICE_ACCOUNT_TOKEN}`;
    const encodedCredentials = encoding.b64encode(credentials);

    return {
        Authorization: `Basic ${encodedCredentials}`
    }
}
