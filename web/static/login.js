function setLoginMessage(text, type) {
    const message = document.getElementById('login-message');
    message.hidden = !text;
    message.textContent = text;
    message.className = text ? `message ${type}` : 'message';
}

async function login(username, password) {
    const response = await fetch('/api/v1/login', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json'
        },
        body: JSON.stringify({ username, password })
    });

    const payload = await response.json().catch(() => ({ error: 'invalid response' }));
    if (!response.ok) {
        throw new Error(payload.error || 'login failed');
    }
}

document.addEventListener('DOMContentLoaded', () => {
    const form = document.getElementById('login-form');
    form.addEventListener('submit', async (event) => {
        event.preventDefault();
        setLoginMessage('', 'info');

        const username = document.getElementById('username').value.trim();
        const password = document.getElementById('password').value;

        try {
            await login(username, password);
            window.location.assign('/');
        } catch (error) {
            setLoginMessage(error.message, 'error');
        }
    });
});
