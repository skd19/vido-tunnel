// Vido Tunnel Control Panel Logic

let pollIntervalId = null;

document.addEventListener('DOMContentLoaded', () => {
    fetchControlStatus();
    startAutoPolling();
    initControlActions();
});

async function fetchControlStatus() {
    try {
        const response = await fetch('/control/status', {
            method: 'POST',
            headers: {
                'X-Requested-With': 'XMLHttpRequest'
            }
        });

        if (response.status === 401) {
            window.location.href = '/login';
            return;
        }

        const data = await response.json();
        updateUI(data);
    } catch (err) {
        console.error('Error fetching control status:', err);
    }
}

function updateUI(data) {
    const statusBadge = document.getElementById('procStatusBadge');
    const pidValue = document.getElementById('procPidValue');
    const portBadge = document.getElementById('portStatusBadge');
    const lastChecked = document.getElementById('lastCheckedTime');
    const statusMsg = document.getElementById('statusMessage');
    const startBtn = document.getElementById('btnStartProc');
    const stopBtn = document.getElementById('btnStopProc');
    const pathBadge = document.getElementById('procPathBadge');

    if (lastChecked) lastChecked.textContent = data.last_checked || '--:--:--';
    if (statusMsg) statusMsg.textContent = data.message || '';

    if (pathBadge) {
        pathBadge.textContent = data.exec_path;
        if (!data.path_exists) {
            pathBadge.classList.add('text-danger');
        } else {
            pathBadge.classList.remove('text-danger');
        }
    }

    if (statusBadge) {
        if (data.running) {
            statusBadge.innerHTML = '<span class="pulse-dot running"></span> RUNNING';
            statusBadge.className = 'badge bg-success-subtle text-success border border-success-subtle fs-6 py-2 px-3';
            if (startBtn) startBtn.disabled = true;
            if (stopBtn) stopBtn.disabled = false;
        } else {
            statusBadge.innerHTML = '<span class="pulse-dot stopped"></span> STOPPED';
            statusBadge.className = 'badge bg-danger-subtle text-danger border border-danger-subtle fs-6 py-2 px-3';
            if (startBtn) startBtn.disabled = !data.path_exists;
            if (stopBtn) stopBtn.disabled = true;
        }
    }

    if (pidValue) {
        pidValue.textContent = data.running && data.pid > 0 ? data.pid : '--';
    }

    if (portBadge) {
        if (data.port_open) {
            portBadge.innerHTML = `<span class="pulse-dot running"></span> PORT ${data.port} OPEN`;
            portBadge.className = 'badge bg-success-subtle text-success border border-success-subtle fs-6 py-2 px-3';
        } else {
            portBadge.innerHTML = `<span class="pulse-dot stopped"></span> PORT ${data.port} CLOSED`;
            portBadge.className = 'badge bg-secondary-subtle text-secondary border border-secondary-subtle fs-6 py-2 px-3';
        }
    }
}

function startAutoPolling() {
    const autoPollCheck = document.getElementById('autoPollCheck');
    if (pollIntervalId) clearInterval(pollIntervalId);

    pollIntervalId = setInterval(() => {
        if (!autoPollCheck || autoPollCheck.checked) {
            fetchControlStatus();
        }
    }, 3000);
}

function initControlActions() {
    const startBtn = document.getElementById('btnStartProc');
    const stopBtn = document.getElementById('btnStopProc');
    const refreshBtn = document.getElementById('btnManualRefresh');
    const actionAlert = document.getElementById('actionAlert');

    if (refreshBtn) {
        refreshBtn.addEventListener('click', () => {
            refreshBtn.innerHTML = '<span class="spinner-border spinner-border-sm"></span>';
            fetchControlStatus().finally(() => {
                setTimeout(() => {
                    refreshBtn.innerHTML = '<i class="bi bi-arrow-clockwise me-1"></i> Refresh';
                }, 400);
            });
        });
    }

    if (startBtn) {
        startBtn.addEventListener('click', async () => {
            if (startBtn.disabled) return;
            startBtn.disabled = true;
            startBtn.innerHTML = '<span class="spinner-border spinner-border-sm me-1"></span> Starting...';
            showAlert('Starting Vidoveo application...', 'info');

            try {
                const response = await fetch('/control/start', {
                    method: 'POST',
                    headers: { 'X-Requested-With': 'XMLHttpRequest' }
                });
                const data = await response.json();

                if (response.ok && data.success) {
                    showAlert(data.message || 'Application started successfully!', 'success');
                } else {
                    showAlert(data.error || 'Failed to start application.', 'danger');
                }
            } catch (err) {
                showAlert('Network error communicating with server.', 'danger');
            } finally {
                startBtn.innerHTML = '<i class="bi bi-play-circle-fill me-1"></i> Start Vidoveo';
                fetchControlStatus();
            }
        });
    }

    if (stopBtn) {
        stopBtn.addEventListener('click', async () => {
            if (stopBtn.disabled) return;
            if (!confirm('Are you sure you want to stop Vidoveo.exe?')) return;

            stopBtn.disabled = true;
            stopBtn.innerHTML = '<span class="spinner-border spinner-border-sm me-1"></span> Stopping...';
            showAlert('Stopping Vidoveo application...', 'info');

            try {
                const response = await fetch('/control/stop', {
                    method: 'POST',
                    headers: { 'X-Requested-With': 'XMLHttpRequest' }
                });
                const data = await response.json();

                if (response.ok && data.success) {
                    showAlert(data.message || 'Application stopped.', 'success');
                } else {
                    showAlert(data.error || 'Failed to stop application.', 'danger');
                }
            } catch (err) {
                showAlert('Network error communicating with server.', 'danger');
            } finally {
                stopBtn.innerHTML = '<i class="bi bi-stop-circle-fill me-1"></i> Stop Vidoveo';
                fetchControlStatus();
            }
        });
    }

    function showAlert(msg, type) {
        if (!actionAlert) return;
        actionAlert.className = `alert alert-${type} alert-dismissible fade show`;
        actionAlert.innerHTML = `${msg} <button type="button" class="btn-close" data-bs-dismiss="alert"></button>`;
        actionAlert.classList.remove('d-none');
    }
}
