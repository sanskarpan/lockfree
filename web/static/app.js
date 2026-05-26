let ws = null;
let reconnectInterval = null;
let session = null;

const stats = {
    stack: 0,
    queue: 0,
    ringbuffer: 0,
    counter: 0,
    list: 0
};

function createEmptyState(message) {
    const emptyState = document.createElement('div');
    emptyState.className = 'empty-state';
    emptyState.textContent = message;
    return emptyState;
}

function replaceChildren(container, children) {
    container.replaceChildren(...children);
}

function renderItems(container, items, className, emptyMessage) {
    if (!Array.isArray(items) || items.length === 0) {
        replaceChildren(container, [createEmptyState(emptyMessage)]);
        return;
    }

    const nodes = items.map((item) => {
        const element = document.createElement('div');
        element.className = className;
        element.textContent = String(item);
        return element;
    });
    replaceChildren(container, nodes);
}

function setGlobalMessage(text, type = 'info') {
    const message = document.getElementById('global-message');
    if (!text) {
        message.hidden = true;
        message.textContent = '';
        message.className = 'message';
        return;
    }
    message.hidden = false;
    message.textContent = text;
    message.className = `message ${type}`;
}

function bindClick(id, handler) {
    const element = document.getElementById(id);
    if (element) {
        element.addEventListener('click', handler);
    }
}

function bindEnterKey(id, handler, requiresWrite = true) {
    const element = document.getElementById(id);
    if (element) {
        element.addEventListener('keydown', (event) => {
            if (event.key === 'Enter' && (!requiresWrite || canWrite())) {
                handler();
            }
        });
    }
}

function bindControls() {
    bindClick('stack-push-btn', stackPush);
    bindClick('stack-pop-btn', stackPop);
    bindClick('stack-clear-btn', stackClear);
    bindEnterKey('stack-input', stackPush);

    bindClick('queue-enqueue-btn', queueEnqueue);
    bindClick('queue-dequeue-btn', queueDequeue);
    bindClick('queue-clear-btn', queueClear);
    bindEnterKey('queue-input', queueEnqueue);

    bindClick('rb-write-btn', rbWrite);
    bindClick('rb-read-btn', rbRead);
    bindClick('rb-clear-btn', rbClear);
    bindEnterKey('rb-input', rbWrite);

    bindClick('counter-inc-btn', counterInc);
    bindClick('counter-dec-btn', counterDec);
    bindClick('counter-add-btn', counterAdd);
    bindClick('counter-reset-btn', counterReset);
    bindEnterKey('counter-input', counterAdd);

    bindClick('list-insert-btn', listInsert);
    bindClick('list-delete-btn', listDelete);
    bindClick('list-search-btn', listSearch);
    bindClick('list-clear-btn', listClear);
    bindEnterKey('list-input', listInsert);

    bindClick('logout-btn', logout);
}

function canWrite() {
    return !session || session.role === 'operator' || session.role === 'admin';
}

function requireWrite(action) {
    if (!canWrite()) {
        setGlobalMessage('Your role is read-only for this tenant.', 'error');
        return false;
    }
    action();
    return true;
}

function applyAccessMode() {
    const badge = document.getElementById('access-mode');
    if (!session) {
        badge.textContent = 'Read/Write';
        badge.className = 'access-badge';
        return;
    }

    const readOnly = !canWrite();
    badge.textContent = readOnly ? 'Read Only' : 'Read/Write';
    badge.className = readOnly ? 'access-badge access-readonly' : 'access-badge access-readwrite';

    document.querySelectorAll('[data-requires-write="true"]').forEach((element) => {
        element.disabled = readOnly;
    });
}

async function loadSession() {
    const response = await fetch('/api/v1/session', { headers: { Accept: 'application/json' } });
    if (response.status === 401) {
        window.location.assign('/login');
        throw new Error('authentication required');
    }
    if (!response.ok) {
        throw new Error('failed to load session');
    }

    const payload = await response.json();
    session = payload.principal;
    document.getElementById('session-user').textContent = session.username;
    document.getElementById('session-role').textContent = session.role;
    document.getElementById('session-tenant').textContent = session.tenant;
    applyAccessMode();
}

async function logout() {
    await fetch('/api/v1/logout', { method: 'POST' });
    window.location.assign('/login');
}

function connect() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws`;
    ws = new WebSocket(wsUrl);

    ws.onopen = () => {
        updateConnectionStatus(true);
        setGlobalMessage('', 'info');
        if (reconnectInterval) {
            clearInterval(reconnectInterval);
            reconnectInterval = null;
        }
        sendMessage({ type: 'getState', operation: 'getState' });
    };

    ws.onclose = () => {
        updateConnectionStatus(false);
        if (!reconnectInterval) {
            reconnectInterval = setInterval(() => {
                connect();
            }, 3000);
        }
    };

    ws.onerror = () => {
        setGlobalMessage('WebSocket error. The client will retry automatically.', 'error');
    };

    ws.onmessage = (event) => {
        let message;
        try {
            message = JSON.parse(event.data);
        } catch (error) {
            setGlobalMessage('Received malformed server response.', 'error');
            return;
        }
        handleMessage(message);
    };
}

function updateConnectionStatus(connected) {
    const indicator = document.getElementById('status-indicator');
    const text = document.getElementById('status-text');
    if (connected) {
        indicator.className = 'status-connected';
        text.textContent = 'Connected';
    } else {
        indicator.className = 'status-disconnected';
        text.textContent = 'Disconnected';
    }
}

function sendMessage(message) {
    if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify(message));
        return;
    }
    setGlobalMessage('WebSocket is not connected.', 'error');
}

function handleMessage(message) {
    switch (message.type) {
        case 'stack':
            updateStackVisualization(message.data);
            if (message.success && message.operation !== 'clear') {
                stats.stack++;
            }
            break;
        case 'queue':
            updateQueueVisualization(message.data);
            if (message.success && message.operation !== 'clear') {
                stats.queue++;
            }
            break;
        case 'ringbuffer':
            updateRingBufferVisualization(message.data);
            if (message.success && message.operation !== 'clear') {
                stats.ringbuffer++;
            }
            break;
        case 'counter':
            updateCounterVisualization(message.data);
            if (message.success && message.operation !== 'reset') {
                stats.counter++;
            }
            break;
        case 'list':
            updateListVisualization(message.data);
            if (message.success && message.operation !== 'search' && message.operation !== 'clear') {
                stats.list++;
            }
            if (message.success && message.operation === 'search') {
                showListMessage('Key found', 'success');
            }
            break;
        case 'state':
            if (message.data.session) {
                session = message.data.session;
                applyAccessMode();
            }
            updateStackVisualization(message.data.stack);
            updateQueueVisualization(message.data.queue);
            updateRingBufferVisualization(message.data.ringbuffer);
            updateCounterVisualization(message.data.counter);
            updateListVisualization(message.data.list);
            break;
        case 'error':
            break;
        default:
            setGlobalMessage(`Unknown message type: ${message.type}`, 'error');
            return;
    }

    if (!message.success && message.error) {
        setGlobalMessage(message.error, 'error');
        if (message.type === 'list') {
            showListMessage(message.error, 'error');
        }
    }

    updateStats();
}

function stackPush() {
    requireWrite(() => {
        const input = document.getElementById('stack-input');
        const value = parseInt(input.value, 10);
        if (!Number.isNaN(value)) {
            sendMessage({ type: 'stack', operation: 'push', value });
            input.value = Math.floor(Math.random() * 100);
        }
    });
}

function stackPop() {
    requireWrite(() => sendMessage({ type: 'stack', operation: 'pop' }));
}

function stackClear() {
    requireWrite(() => sendMessage({ type: 'stack', operation: 'clear' }));
}

function updateStackVisualization(data) {
    const container = document.getElementById('stack-viz');
    document.getElementById('stack-length').textContent = data.length;
    renderItems(container, data.items, 'stack-item', 'Stack is empty');
}

function queueEnqueue() {
    requireWrite(() => {
        const input = document.getElementById('queue-input');
        const value = parseInt(input.value, 10);
        if (!Number.isNaN(value)) {
            sendMessage({ type: 'queue', operation: 'enqueue', value });
            input.value = Math.floor(Math.random() * 100);
        }
    });
}

function queueDequeue() {
    requireWrite(() => sendMessage({ type: 'queue', operation: 'dequeue' }));
}

function queueClear() {
    requireWrite(() => sendMessage({ type: 'queue', operation: 'clear' }));
}

function updateQueueVisualization(data) {
    const container = document.querySelector('.queue-items');
    document.getElementById('queue-length').textContent = data.length;
    renderItems(container, data.items, 'queue-item', 'Queue is empty');
}

function rbWrite() {
    requireWrite(() => {
        const input = document.getElementById('rb-input');
        const value = parseInt(input.value, 10);
        if (!Number.isNaN(value)) {
            sendMessage({ type: 'ringbuffer', operation: 'write', value });
            input.value = Math.floor(Math.random() * 100);
        }
    });
}

function rbRead() {
    requireWrite(() => sendMessage({ type: 'ringbuffer', operation: 'read' }));
}

function rbClear() {
    requireWrite(() => sendMessage({ type: 'ringbuffer', operation: 'clear' }));
}

function updateRingBufferVisualization(data) {
    document.getElementById('rb-length').textContent = data.length;
    document.getElementById('rb-capacity').textContent = data.capacity;
    document.getElementById('rb-available').textContent = data.available;
    drawRingBuffer(data);
}

function drawRingBuffer(data) {
    const svg = document.getElementById('rb-svg');
    const capacity = data.capacity;
    const filled = data.length;
    const centerX = 200;
    const centerY = 200;
    const radius = 120;
    svg.replaceChildren();

    const angleStep = (2 * Math.PI) / capacity;
    for (let i = 0; i < capacity; i++) {
        const angle = i * angleStep - Math.PI / 2;
        const x = centerX + radius * Math.cos(angle);
        const y = centerY + radius * Math.sin(angle);
        const isFilled = i < filled;

        const circle = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
        circle.setAttribute('cx', x);
        circle.setAttribute('cy', y);
        circle.setAttribute('r', 20);
        circle.setAttribute('class', isFilled ? 'rb-slot-filled' : 'rb-slot-empty');
        svg.appendChild(circle);

        const text = document.createElementNS('http://www.w3.org/2000/svg', 'text');
        text.setAttribute('x', x);
        text.setAttribute('y', y + 5);
        text.setAttribute('class', 'rb-text');
        text.textContent = i;
        svg.appendChild(text);
    }

    const centerText = document.createElementNS('http://www.w3.org/2000/svg', 'text');
    centerText.setAttribute('x', centerX);
    centerText.setAttribute('y', centerY);
    centerText.setAttribute('class', 'rb-text');
    centerText.setAttribute('font-size', '24');
    centerText.textContent = `${filled}/${capacity}`;
    svg.appendChild(centerText);

    const labelFill = document.createElementNS('http://www.w3.org/2000/svg', 'text');
    labelFill.setAttribute('x', centerX);
    labelFill.setAttribute('y', centerY + 30);
    labelFill.setAttribute('class', 'rb-text');
    labelFill.setAttribute('font-size', '14');
    labelFill.textContent = `Available: ${data.available}`;
    svg.appendChild(labelFill);
}

function counterInc() {
    requireWrite(() => sendMessage({ type: 'counter', operation: 'inc' }));
}

function counterDec() {
    requireWrite(() => sendMessage({ type: 'counter', operation: 'dec' }));
}

function counterAdd() {
    requireWrite(() => {
        const input = document.getElementById('counter-input');
        const value = parseInt(input.value, 10);
        if (!Number.isNaN(value)) {
            sendMessage({ type: 'counter', operation: 'add', value });
        }
    });
}

function counterReset() {
    requireWrite(() => sendMessage({ type: 'counter', operation: 'reset' }));
}

function updateCounterVisualization(data) {
    const value = data.value;
    document.getElementById('counter-value').textContent = value;
    document.getElementById('counter-display').textContent = value;
    const percentage = Math.min(Math.max((value / 100) * 100, 0), 100);
    document.getElementById('counter-bar-fill').style.width = `${percentage}%`;
}

function listInsert() {
    requireWrite(() => {
        const input = document.getElementById('list-input');
        const value = parseInt(input.value, 10);
        if (!Number.isNaN(value)) {
            sendMessage({ type: 'list', operation: 'insert', value });
            input.value = Math.floor(Math.random() * 100);
        }
    });
}

function listDelete() {
    requireWrite(() => {
        const input = document.getElementById('list-input');
        const value = parseInt(input.value, 10);
        if (!Number.isNaN(value)) {
            sendMessage({ type: 'list', operation: 'delete', value });
        }
    });
}

function listSearch() {
    const input = document.getElementById('list-input');
    const value = parseInt(input.value, 10);
    if (!Number.isNaN(value)) {
        sendMessage({ type: 'list', operation: 'search', value });
    }
}

function listClear() {
    requireWrite(() => sendMessage({ type: 'list', operation: 'clear' }));
}

function updateListVisualization(data) {
    const container = document.getElementById('list-viz');
    document.getElementById('list-length').textContent = data.length;
    renderItems(container, data.items, 'list-item', 'List is empty');
}

function showListMessage(text, type) {
    const messageDiv = document.getElementById('list-message');
    messageDiv.textContent = text;
    messageDiv.className = text ? `message ${type}` : 'message';
    window.setTimeout(() => {
        messageDiv.textContent = '';
        messageDiv.className = 'message';
    }, 3000);
}

function updateStats() {
    document.getElementById('stack-ops').textContent = stats.stack;
    document.getElementById('queue-ops').textContent = stats.queue;
    document.getElementById('rb-ops').textContent = stats.ringbuffer;
    document.getElementById('counter-ops').textContent = stats.counter;
    document.getElementById('list-ops').textContent = stats.list;
}

document.addEventListener('DOMContentLoaded', async () => {
    bindControls();
    updateStats();
    try {
        await loadSession();
        connect();
    } catch (error) {
        setGlobalMessage(error.message, 'error');
    }
});
