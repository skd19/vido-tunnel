// Vido Tunnel Client Application Logic

document.addEventListener('DOMContentLoaded', () => {
    initSearchFilter();
    initRenameModal();
});

// Navigate directory via POST
function navigateToDir(relPath) {
    const form = document.createElement('form');
    form.method = 'POST';
    form.action = '/browse';

    const input = document.createElement('input');
    input.type = 'hidden';
    input.name = 'path';
    input.value = relPath;

    form.appendChild(input);
    document.body.appendChild(form);
    form.submit();
}

// Download single file via POST
function downloadFile(relPath) {
    const form = document.createElement('form');
    form.method = 'POST';
    form.action = '/download';

    const input = document.createElement('input');
    input.type = 'hidden';
    input.name = 'path';
    input.value = relPath;

    form.appendChild(input);
    document.body.appendChild(form);
    form.submit();
    document.body.removeChild(form);
}

// Download folder as ZIP via POST
function downloadFolderZip(relPath) {
    const form = document.createElement('form');
    form.method = 'POST';
    form.action = '/download/zip';

    const input = document.createElement('input');
    input.type = 'hidden';
    input.name = 'path';
    input.value = relPath;

    form.appendChild(input);
    document.body.appendChild(form);
    form.submit();
    document.body.removeChild(form);
}

// Search filter in file list
function initSearchFilter() {
    const searchInput = document.getElementById('explorerSearch');
    if (!searchInput) return;

    searchInput.addEventListener('input', (e) => {
        const query = e.target.value.toLowerCase().trim();
        const rows = document.querySelectorAll('.explorer-row');
        let visibleCount = 0;

        rows.forEach(row => {
            const itemName = row.getAttribute('data-name')?.toLowerCase() || '';
            if (itemName.includes(query)) {
                row.style.display = '';
                visibleCount++;
            } else {
                row.style.display = 'none';
            }
        });

        const noMatchEl = document.getElementById('noMatchRow');
        if (noMatchEl) {
            noMatchEl.style.display = (visibleCount === 0 && rows.length > 0) ? '' : 'none';
        }
    });
}

// Rename Modal Handling
function initRenameModal() {
    const renameModalEl = document.getElementById('renameModal');
    if (!renameModalEl) return;

    const modal = new bootstrap.Modal(renameModalEl);
    const form = document.getElementById('renameForm');
    const folderPathInput = document.getElementById('renameFolderPath');
    const folderNameInput = document.getElementById('renameFolderName');
    const currentNameSpan = document.getElementById('renameCurrentName');
    const errorAlert = document.getElementById('renameErrorAlert');
    const submitBtn = document.getElementById('renameSubmitBtn');

    window.openRenameModal = function(relPath, currentName) {
        folderPathInput.value = relPath;
        folderNameInput.value = currentName;
        currentNameSpan.textContent = currentName;
        errorAlert.classList.add('d-none');
        errorAlert.textContent = '';
        modal.show();
        setTimeout(() => folderNameInput.focus(), 250);
    };

    if (form) {
        form.addEventListener('submit', async (e) => {
            e.preventDefault();
            const relPath = folderPathInput.value;
            const newName = folderNameInput.value.trim();

            if (!newName) {
                errorAlert.textContent = 'Please enter a valid folder name.';
                errorAlert.classList.remove('d-none');
                return;
            }

            submitBtn.disabled = true;
            submitBtn.innerHTML = '<span class="spinner-border spinner-border-sm me-1" role="status"></span> Renaming...';
            errorAlert.classList.add('d-none');

            try {
                const formData = new URLSearchParams();
                formData.append('path', relPath);
                formData.append('new_name', newName);

                const response = await fetch('/rename', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/x-www-form-urlencoded',
                        'X-Requested-With': 'XMLHttpRequest'
                    },
                    body: formData.toString()
                });

                const data = await response.json();

                if (response.ok && data.success) {
                    modal.hide();
                    // Reload the parent directory to reflect changes
                    const currentDir = document.getElementById('currentRelPath')?.value || '';
                    navigateToDir(currentDir);
                } else {
                    errorAlert.textContent = data.error || 'Failed to rename folder.';
                    errorAlert.classList.remove('d-none');
                }
            } catch (err) {
                errorAlert.textContent = 'Network or server error occurred.';
                errorAlert.classList.remove('d-none');
            } finally {
                submitBtn.disabled = false;
                submitBtn.innerHTML = '<i class="bi bi-check-lg me-1"></i> Save Changes';
            }
        });
    }
}
