async function loadProjects() {
  const sel = document.getElementById('project-select');
  if (!sel) return;

  const res = await fetch('/api/projects');
  if (!res.ok) return;

  const data = await res.json();
  sel.innerHTML = '';
  for (const name of data.names || []) {
    const opt = document.createElement('option');
    opt.value = name;
    opt.textContent = name;
    if (name === data.active) opt.selected = true;
    sel.appendChild(opt);
  }
}

async function switchProject(name) {
  const res = await fetch(`/api/projects/${encodeURIComponent(name)}/switch`, { method: 'POST' });
  if (!res.ok) {
    let error = '切換失敗';
    try {
      const data = await res.json();
      error = data.error || error;
    } catch (_) {
    }
    alert(error);
    await loadProjects();
    return;
  }
  location.reload();
}

async function promptCreateProject() {
  const name = prompt('新增專案名稱（英數、-、_，最長 64 字元）：');
  if (!name) return;

  const res = await fetch('/api/projects', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  });
  if (!res.ok) {
    let error = '建立失敗';
    try {
      const data = await res.json();
      error = data.error || error;
    } catch (_) {
    }
    alert('建立失敗：' + error);
    return;
  }
  await switchProject(name);
}

document.addEventListener('DOMContentLoaded', loadProjects);
