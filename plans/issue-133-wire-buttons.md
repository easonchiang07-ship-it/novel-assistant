# Issue #133：將 toast-only 按鈕接上實際邏輯

狀態：計畫中，尚未實作

Branch: `feat/issue-133-wire-buttons`

---

## 盤點結果

### ✅ 可直接串接（有 API）

| 按鈕 | 位置 | API | 請求格式 |
|---|---|---|---|
| 重新索引 | page-overview topbar | `POST /ingest` | 無 body |
| 建立備份 | page-history topbar | `POST /api/backups/create` | 無 body |
| 匯出全本報告 | page-overview topbar | `POST /api/manuscript/export` | `{}` (空 body 用 default) |
| history 匯出 | page-history table | `POST /api/history/export` | `{"ids":["<id>"]}` |
| history 刪除 | page-history table | `POST /api/history/delete` | `{"id":"<id>"}` → 成功後 reload |

### ❌ 無 API / 需 modal（改為 disabled）

| 按鈕 | 原因 |
|---|---|
| 接受全部修改 / 接受此修改 / 回填編輯器 / 略過 | 無後端 API |
| + 新增角色 / + 新增設定 / + 新增風格 / + 手動新增伏筆 | 需 modal 表單，另開 issue |
| 匯出關係圖 / 匯出時間軸 | 無後端 API |
| 切換專案 | 需 project list modal，另開 issue |
| 章節列表「報告」按鈕 | 需傳 chapter bundle，格式複雜，另開 issue |
| 查看 diff（history table 靜態列） | 需路由 + 動態載入，另開 issue |

---

## 實作步驟

### Step 1：連接 page-history 表格到真實資料

`page-history` 目前仍是 hardcoded demo（5 筆）。
`.History` 已由 `handleNovelAssistantPage` 傳入 template（`[]*reviewhistory.Entry`）。

把 `<tbody>` 內的 5 個 hardcoded `<tr>` 換成：

```html
{{range .History}}
<tr data-id="{{.ID}}">
  <td>{{.ChapterTitle}}{{if .SceneTitle}} · {{.SceneTitle}}{{end}}</td>
  <td>#{{.KindVersion}}</td>
  <td>{{timeAgo .CreatedAt}}</td>
  <td style="font-family:var(--font-mono);font-size:11px">{{index .Styles 0 | default "—"}}</td>
  <td><span class="badge badge-ghost">{{.Kind}}</span></td>
  <td>
    <div class="flex gap-2">
      <button class="btn" style="padding:3px 7px;font-size:11px"
        onclick="exportHistory('{{.ID}}')">匯出</button>
      <button class="btn btn-danger" style="padding:3px 7px;font-size:11px"
        onclick="deleteHistory('{{.ID}}')">刪除</button>
    </div>
  </td>
</tr>
{{end}}
{{if not .History}}
<tr><td colspan="6" style="text-align:center;color:var(--text-ghost);padding:20px">尚無審查記錄</td></tr>
{{end}}
```

注意：`{{index .Styles 0 | default "—"}}` 需要在 FuncMap 加 `"default"` helper，或改用 `{{if .Styles}}{{index .Styles 0}}{{else}}—{{end}}`。

### Step 2：實作有 API 的 JS 函數

在 `<script>` 區塊（`</script>` 前）加入：

```javascript
/* ── 重新索引 ── */
async function doReindex(btn) {
  btn.disabled = true;
  toast('重新索引中…');
  try {
    const resp = await fetch('/ingest', { method: 'POST' });
    const data = await resp.json();
    toast(resp.ok ? (data.message || '索引完成') : ('索引失敗：' + (data.error || resp.status)));
  } catch (e) {
    toast('索引失敗：網路錯誤');
  } finally {
    btn.disabled = false;
  }
}

/* ── 建立備份 ── */
async function doBackup(btn) {
  btn.disabled = true;
  try {
    const resp = await fetch('/api/backups/create', { method: 'POST' });
    const data = await resp.json();
    toast(resp.ok ? (data.message || '備份已建立') : ('備份失敗：' + (data.error || resp.status)));
  } catch (e) {
    toast('備份失敗：網路錯誤');
  } finally {
    btn.disabled = false;
  }
}

/* ── 匯出全本 ── */
async function exportManuscript(btn) {
  btn.disabled = true;
  toast('匯出中…');
  try {
    const resp = await fetch('/api/manuscript/export', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: '{}'
    });
    if (!resp.ok) {
      const data = await resp.json().catch(() => ({}));
      toast('匯出失敗：' + (data.error || resp.status));
      return;
    }
    const blob = await resp.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url; a.download = 'manuscript.md'; a.click();
    URL.revokeObjectURL(url);
    toast('匯出完成');
  } catch (e) {
    toast('匯出失敗：網路錯誤');
  } finally {
    btn.disabled = false;
  }
}

/* ── history 匯出 ── */
async function exportHistory(id) {
  const resp = await fetch('/api/history/export', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ids: [id] })
  });
  if (!resp.ok) { toast('匯出失敗'); return; }
  const blob = await resp.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a'); a.href = url; a.download = 'history.md'; a.click();
  URL.revokeObjectURL(url);
  toast('匯出完成');
}

/* ── history 刪除 ── */
async function deleteHistory(id) {
  if (!confirm('確定刪除此筆審查記錄？')) return;
  const resp = await fetch('/api/history/delete', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id })
  });
  if (resp.ok) { toast('已刪除'); location.reload(); }
  else { const d = await resp.json().catch(() => ({})); toast('刪除失敗：' + (d.error || resp.status)); }
}
```

### Step 3：更新按鈕 onclick

| 按鈕 | 改法 |
|---|---|
| 重新索引 | `onclick="doReindex(this)"` |
| 建立備份 | `onclick="doBackup(this)"` |
| 匯出全本報告 | `onclick="exportManuscript(this)"` |

### Step 4：Disable 無 API 按鈕

```html
<!-- 接受全部修改 -->
<button class="btn btn-primary" disabled title="即將推出">接受全部修改</button>

<!-- 接受此修改 / 回填 / 略過 -->
<button class="btn btn-primary" disabled title="即將推出">接受此修改</button>
<button class="btn" disabled title="即將推出">回填編輯器</button>
<button class="btn btn-danger" disabled title="即將推出">略過</button>

<!-- + 新增類按鈕 -->
<button class="btn btn-primary" disabled title="即將推出">+ 新增角色</button>
<!-- 同上套用於 + 新增設定、+ 新增風格、+ 手動新增伏筆 -->

<!-- 匯出關係圖 / 時間軸 -->
<button class="btn" disabled title="即將推出">匯出</button>

<!-- 切換專案 -->
<button class="project-btn" disabled title="即將推出">...</button>
```

### Step 5：章節報告按鈕

暫時改為 `disabled title="即將推出"`，等 `/api/chapter-report/export` 整合後另開 issue 實作。

---

## 驗收條件確認

- [ ] 所有 toast-only 按鈕都有盤點結果（本文件）
- [ ] 重新索引、建立備份、匯出全本 真正觸發後端
- [ ] history 刪除後頁面 reload，匯出觸發下載
- [ ] 無 API 的按鈕 `disabled`，不再假裝操作成功
- [ ] `go build ./...` 零錯誤
- [ ] CI test / desktop-build / pr-size 通過
