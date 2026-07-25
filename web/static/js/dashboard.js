// Dashboard behaviour for index.html. This file is served from /static so the
// Content-Security-Policy can forbid inline scripts entirely.

function showTab(tabId, trigger) {
    document.querySelectorAll('.tab-content').forEach(t => t.classList.remove('active'));
    document.querySelectorAll('.sidebar-nav a').forEach(a => a.classList.remove('active'));
    const tab = document.getElementById(tabId);
    if (tab) tab.classList.add('active');
    if (trigger) trigger.classList.add('active');
}

function getCsrfToken() {
    const input = document.querySelector('input[name="csrf_token"]');
    return input ? input.value : '';
}

async function sendNow() {
    if (!confirm("Send newsletter now?")) return;

    const csrfToken = getCsrfToken();
    if (!csrfToken) {
        alert("Could not find a CSRF token on this page. Reload and try again.");
        return;
    }

    let resp;
    try {
        resp = await fetch('/send', {
            method: 'POST',
            headers: { 'X-CSRF-Token': csrfToken }
        });
    } catch (e) {
        alert("Error sending newsletter: the request could not be completed.");
        return;
    }

    let data = null;
    try {
        data = await resp.json();
    } catch (e) {
        // Non-JSON body (e.g. a plain-text error page); handled below.
    }

    if (!resp.ok) {
        alert("Error sending newsletter" + (data && data.error ? ": " + data.error : "."));
        return;
    }

    if (data && data.status === 'Skipped') {
        alert("Nothing was sent: " + (data.message || "no recommendations available."));
        return;
    }

    if (data && data.status === 'Sent') {
        const count = typeof data.recipients === 'number' ? data.recipients : null;
        alert(count === null
            ? "Newsletter queued for delivery."
            : "Newsletter queued for delivery to " + count + " recipient(s).");
        return;
    }

    alert("Unexpected response from the server.");
}

function fetchLogs() {
    fetch('/api/logs')
        .then(resp => resp.json())
        .then(data => {
            const logDiv = document.getElementById('live-log');
            if (!logDiv) return;
            if (data.logs && data.logs.length > 0) {
                logDiv.textContent = data.logs.map(entry => {
                    const time = new Date(entry.time).toLocaleTimeString();
                    return '[' + entry.level.toUpperCase() + '] ' + time + ' - ' + entry.message;
                }).join('\n');
                logDiv.scrollTop = logDiv.scrollHeight;
            } else {
                logDiv.textContent = 'No log entries available. Logs are available in the server console.';
            }
        })
        .catch(() => {
            const logDiv = document.getElementById('live-log');
            if (logDiv) logDiv.textContent = 'Logs are available in the server console.';
        });
}

document.addEventListener('DOMContentLoaded', () => {
    document.querySelectorAll('[data-tab]').forEach(link => {
        link.addEventListener('click', event => {
            event.preventDefault();
            showTab(link.dataset.tab, link);
        });
    });

    const sendBtn = document.getElementById('send-now');
    if (sendBtn) sendBtn.addEventListener('click', sendNow);

    fetchLogs();
    setInterval(fetchLogs, 5000);
});
