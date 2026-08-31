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

    const appData = data.app || data;
    const tunnelData = data.tunnel;

    if (lastChecked) lastChecked.textContent = appData.last_checked || '--:--:--';
    if (statusMsg) statusMsg.textContent = appData.message || '';

    if (pathBadge) {
        pathBadge.textContent = appData.exec_path;
        if (!appData.path_exists) {
            pathBadge.classList.add('text-danger');
        } else {
            pathBadge.classList.remove('text-danger');
        }
    }

    if (statusBadge) {
        if (appData.running) {
            statusBadge.innerHTML = '<span class="pulse-dot running"></span> RUNNING';
            statusBadge.className = 'badge bg-success-subtle text-success border border-success-subtle fs-6 py-2 px-3';
            if (startBtn) startBtn.disabled = true;
            if (stopBtn) stopBtn.disabled = false;
        } else {
            statusBadge.innerHTML = '<span class="pulse-dot stopped"></span> STOPPED';
            statusBadge.className = 'badge bg-danger-subtle text-danger border border-danger-subtle fs-6 py-2 px-3';
            if (startBtn) startBtn.disabled = !appData.path_exists;
            if (stopBtn) stopBtn.disabled = true;
        }
    }

    if (pidValue) {
        pidValue.textContent = appData.running && appData.pid > 0 ? appData.pid : '--';
    }

    if (portBadge) {
        if (appData.port_open) {
            portBadge.innerHTML = `<span class="pulse-dot running"></span> PORT ${appData.port} OPEN`;
            portBadge.className = 'badge bg-success-subtle text-success border border-success-subtle fs-6 py-2 px-3';
        } else {
            portBadge.innerHTML = `<span class="pulse-dot stopped"></span> PORT ${appData.port} CLOSED`;
            portBadge.className = 'badge bg-secondary-subtle text-secondary border border-secondary-subtle fs-6 py-2 px-3';
        }
    }

    // Update Tunnel UI if present
    if (tunnelData) {
        const tunnelBadge = document.getElementById('tunnelStatusBadge');
        const tunnelBinary = document.getElementById('tunnelBinaryBadge');
        const tunnelName = document.getElementById('tunnelNameValue');
        const tunnelPid = document.getElementById('tunnelPidValue');
        const tunnelMsg = document.getElementById('tunnelMessage');
        const restartTunnelBtn = document.getElementById('btnRestartTunnel');
        const startTunnelBtn = document.getElementById('btnStartTunnel');
        const stopTunnelBtn = document.getElementById('btnStopTunnel');

        if (tunnelName) tunnelName.textContent = tunnelData.tunnel_name;
        if (tunnelPid) tunnelPid.textContent = tunnelData.running && tunnelData.pid > 0 ? tunnelData.pid : '--';
        if (tunnelMsg) tunnelMsg.textContent = tunnelData.message;

        if (tunnelBinary) {
            tunnelBinary.textContent = tunnelData.binary_path || 'cloudflared not detected in PATH';
            if (!tunnelData.installed) {
                tunnelBinary.classList.add('text-danger');
            } else {
                tunnelBinary.classList.remove('text-danger');
            }
        }

        if (tunnelBadge) {
            if (tunnelData.running) {
                tunnelBadge.innerHTML = '<span class="pulse-dot running"></span> TUNNEL RUNNING';
                tunnelBadge.className = 'badge bg-success-subtle text-success border border-success-subtle fs-6 py-2 px-3';
                if (startTunnelBtn) startTunnelBtn.disabled = true;
                if (restartTunnelBtn) restartTunnelBtn.disabled = false;
                if (stopTunnelBtn) stopTunnelBtn.disabled = false;
            } else if (tunnelData.installed) {
                tunnelBadge.innerHTML = '<span class="pulse-dot stopped"></span> TUNNEL STOPPED';
                tunnelBadge.className = 'badge bg-secondary-subtle text-secondary border border-secondary-subtle fs-6 py-2 px-3';
                if (startTunnelBtn) startTunnelBtn.disabled = false;
                if (restartTunnelBtn) restartTunnelBtn.disabled = false;
                if (stopTunnelBtn) stopTunnelBtn.disabled = true;
            } else {
                tunnelBadge.innerHTML = '<span class="pulse-dot warning"></span> NOT INSTALLED';
                tunnelBadge.className = 'badge bg-warning-subtle text-warning border border-warning-subtle fs-6 py-2 px-3';
                if (startTunnelBtn) startTunnelBtn.disabled = true;
                if (restartTunnelBtn) restartTunnelBtn.disabled = true;
                if (stopTunnelBtn) stopTunnelBtn.disabled = true;
            }
        }
    }
}

function startAutoPolling() {
    const autoPollCheck = document.getElementById('autoPollCheck');
    if (pollIntervalId) clearInterval(pollIntervalId);

    if (autoPollCheck) {
        autoPollCheck.addEventListener('change', () => {
            if (autoPollCheck.checked) {
                fetchControlStatus();
            }
        });
    }

    pollIntervalId = setInterval(() => {
        if (autoPollCheck && autoPollCheck.checked) {
            fetchControlStatus();
        }
    }, 3000);
}

function initControlActions() {
    const startBtn = document.getElementById('btnStartProc');
    const stopBtn = document.getElementById('btnStopProc');
    const startTunnelBtn = document.getElementById('btnStartTunnel');
    const restartTunnelBtn = document.getElementById('btnRestartTunnel');
    const stopTunnelBtn = document.getElementById('btnStopTunnel');
    const refreshBtn = document.getElementById('btnManualRefresh');
    const actionAlert = document.getElementById('actionAlert');

    const exitBtn = document.getElementById('btnExitApp');
    const confirmExitBtn = document.getElementById('btnConfirmExit');
    const shutdownPcToggle = document.getElementById('shutdownPcToggle');
    const modalShutdownNotice = document.getElementById('modalShutdownNotice');
    let exitModal = null;
    const modalEl = document.getElementById('exitConfirmModal');
    if (modalEl && typeof bootstrap !== 'undefined') {
        exitModal = new bootstrap.Modal(modalEl);
    }

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

    if (startTunnelBtn) {
        startTunnelBtn.addEventListener('click', async () => {
            if (startTunnelBtn.disabled) return;
            startTunnelBtn.disabled = true;
            startTunnelBtn.innerHTML = '<span class="spinner-border spinner-border-sm me-1"></span> Starting...';
            showAlert('Starting Cloudflare tunnel...', 'info');

            try {
                const response = await fetch('/control/tunnel/start', {
                    method: 'POST',
                    headers: { 'X-Requested-With': 'XMLHttpRequest' }
                });
                const data = await response.json();

                if (response.ok && data.success) {
                    showAlert(data.message || 'Cloudflare tunnel started successfully!', 'success');
                } else {
                    showAlert(data.error || 'Failed to start Cloudflare tunnel.', 'danger');
                }
            } catch (err) {
                showAlert('Network error communicating with server.', 'danger');
            } finally {
                startTunnelBtn.innerHTML = '<i class="bi bi-play-circle-fill me-1"></i> Start Tunnel';
                fetchControlStatus();
            }
        });
    }

    if (restartTunnelBtn) {
        restartTunnelBtn.addEventListener('click', async () => {
            if (restartTunnelBtn.disabled) return;
            restartTunnelBtn.disabled = true;
            restartTunnelBtn.innerHTML = '<span class="spinner-border spinner-border-sm me-1"></span> Restarting...';
            showAlert('Restarting Cloudflare tunnel...', 'info');

            try {
                const response = await fetch('/control/tunnel/restart', {
                    method: 'POST',
                    headers: { 'X-Requested-With': 'XMLHttpRequest' }
                });
                const data = await response.json();

                if (response.ok && data.success) {
                    showAlert(data.message || 'Cloudflare tunnel restarted successfully!', 'success');
                } else {
                    showAlert(data.error || 'Failed to restart Cloudflare tunnel.', 'danger');
                }
            } catch (err) {
                showAlert('Network error communicating with server.', 'danger');
            } finally {
                restartTunnelBtn.innerHTML = '<i class="bi bi-arrow-repeat me-1"></i> Restart Tunnel';
                fetchControlStatus();
            }
        });
    }

    if (stopTunnelBtn) {
        stopTunnelBtn.addEventListener('click', async () => {
            if (stopTunnelBtn.disabled) return;
            if (!confirm('Are you sure you want to stop the Cloudflare tunnel?')) return;

            stopTunnelBtn.disabled = true;
            stopTunnelBtn.innerHTML = '<span class="spinner-border spinner-border-sm me-1"></span> Stopping...';
            showAlert('Stopping Cloudflare tunnel...', 'info');

            try {
                const response = await fetch('/control/tunnel/stop', {
                    method: 'POST',
                    headers: { 'X-Requested-With': 'XMLHttpRequest' }
                });
                const data = await response.json();

                if (response.ok && data.success) {
                    showAlert(data.message || 'Cloudflare tunnel stopped.', 'success');
                } else {
                    showAlert(data.error || 'Failed to stop Cloudflare tunnel.', 'danger');
                }
            } catch (err) {
                showAlert('Network error communicating with server.', 'danger');
            } finally {
                stopTunnelBtn.innerHTML = '<i class="bi bi-stop-circle-fill me-1"></i> Stop Tunnel';
                fetchControlStatus();
            }
        });
    }

    if (exitBtn) {
        exitBtn.addEventListener('click', () => {
            if (shutdownPcToggle && modalShutdownNotice) {
                if (shutdownPcToggle.checked) {
                    modalShutdownNotice.classList.remove('d-none');
                } else {
                    modalShutdownNotice.classList.add('d-none');
                }
            }
            if (exitModal) {
                exitModal.show();
            } else if (confirm('Are you sure you want to close Vido Tunnel?')) {
                performAppExit();
            }
        });
    }

    if (shutdownPcToggle) {
        shutdownPcToggle.addEventListener('change', () => {
            if (modalShutdownNotice) {
                if (shutdownPcToggle.checked) {
                    modalShutdownNotice.classList.remove('d-none');
                } else {
                    modalShutdownNotice.classList.add('d-none');
                }
            }
        });
    }

    if (confirmExitBtn) {
        confirmExitBtn.addEventListener('click', () => {
            if (exitModal) exitModal.hide();
            performAppExit();
        });
    }

    async function performAppExit() {
        const shutdownPC = shutdownPcToggle && shutdownPcToggle.checked;
        showAlert('Closing Vido Tunnel and terminating all services...', 'warning');

        try {
            const formData = new URLSearchParams();
            formData.set('shutdown_pc', shutdownPC ? 'true' : 'false');

            await fetch('/control/app/exit', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/x-www-form-urlencoded',
                    'X-Requested-With': 'XMLHttpRequest'
                },
                body: formData.toString()
            });

            document.body.innerHTML = `
                <div class="d-flex align-items-center justify-content-center vh-100 bg-black text-light font-monospace text-center p-4">
                    <div>
                        <h2 class="text-danger mb-3"><i class="bi bi-power"></i> Application Closed</h2>
                        <p class="text-muted">Vido Tunnel has been shut down cleanly and processes terminated.</p>
                        ${shutdownPC ? '<p class="text-danger fw-bold">Computer will shut down in a few seconds...</p>' : '<p class="text-muted small">You can now close this browser tab.</p>'}
                    </div>
                </div>
            `;
        } catch (err) {
            showAlert('Server closed.', 'secondary');
        }
    }

    function showAlert(msg, type) {
        if (!actionAlert) return;
        actionAlert.className = `alert alert-${type} alert-dismissible fade show`;
        actionAlert.innerHTML = `${msg} <button type="button" class="btn-close" data-bs-dismiss="alert"></button>`;
        actionAlert.classList.remove('d-none');
    }
}
