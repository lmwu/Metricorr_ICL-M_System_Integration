async function triggerMeasure() {
    const btn = document.getElementById('btnTrigger');
    btn.disabled = true;
    btn.innerText = '下發指令中...';

    try {
        const res = await fetch('/api/v1/trigger', { method: 'POST' });
        const data = await res.json();
        if (res.ok) {
            alert('手動測量指令已下發！請等待設備測量完畢後點擊讀取。');
        } else {
            alert('錯誤: ' + data.message);
        }
    } catch (e) {
        alert('網路請求失敗: ' + e);
    } finally {
        btn.disabled = false;
        btn.innerText = '觸發單次測量';
    }
}

async function fetchData() {
    const btn = document.getElementById('btnRefresh');
    btn.disabled = true;

    try {
        const res = await fetch('/api/v1/data');
        if (!res.ok) throw new Error(await res.text());

        const data = await res.json();
        document.getElementById('thickness').innerText = data.thickness_1.toFixed(1);
        document.getElementById('metalLoss').innerText = data.metal_loss_1.toFixed(2);
        document.getElementById('eoff').innerText = (data.e_off * 1000).toFixed(1); // V 轉 mV
        document.getElementById('jac').innerText = data.j_ac.toFixed(2);
        document.getElementById('timestamp').innerText = '最後更新: ' + data.timestamp;
    } catch (e) {
        alert('取得數據失敗: ' + e.message);
    } finally {
        btn.disabled = false;
    }
}

// 頁面載入時自動拉取一次
window.onload = fetchData;