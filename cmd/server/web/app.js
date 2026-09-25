async function fetchStations() {
    try {
        const res = await fetch('/api/v1/stations');
        const data = await res.json();
        const container = document.getElementById('stationCards');
        container.innerHTML = '';

        if (Object.keys(data).length === 0) {
            container.innerHTML = `<div class="col-span-full p-8 text-center bg-white rounded-xl border text-slate-400">目前尚無已連線之 4G 網關。請啟動 LTE 網關透傳腳本。</div>`;
            return;
        }

        for (const [stID, st] of Object.entries(data)) {
            const ch1 = st.channel_1 || {};
            const cardHtml = `
            <div class="bg-white p-5 rounded-xl border border-slate-200 shadow-sm hover:shadow-md transition">
                <div class="flex justify-between items-start border-b pb-3 mb-3">
                    <div>
                        <span class="text-xs font-bold text-blue-600 bg-blue-50 px-2 py-0.5 rounded">測站 ID</span>
                        <h3 class="text-lg font-bold text-slate-800 mt-1">${stID}</h3>
                    </div>
                    <span class="text-[10px] text-slate-400">${st.timestamp}</span>
                </div>

                <div class="grid grid-cols-2 gap-3 mb-4 text-xs">
                    <div class="bg-slate-50 p-2.5 rounded">
                        <span class="text-slate-500 block">Ch1 斷電電位 Eoff</span>
                        <span class="text-base font-bold text-slate-800">${(ch1.e_off * 1000).toFixed(1)} mV</span>
                    </div>
                    <div class="bg-slate-50 p-2.5 rounded">
                        <span class="text-slate-500 block">AC 干擾密度 Jac</span>
                        <span class="text-base font-bold ${ch1.j_ac > 30 ? 'text-red-600' : 'text-slate-800'}">${ch1.j_ac.toFixed(2)} A/m²</span>
                    </div>
                    <div class="bg-slate-50 p-2.5 rounded">
                        <span class="text-slate-500 block">ER 探頭剩餘厚度</span>
                        <span class="text-base font-bold text-slate-800">${ch1.thickness.toFixed(1)} µm</span>
                    </div>
                    <div class="bg-slate-50 p-2.5 rounded">
                        <span class="text-slate-500 block">累積金屬損失</span>
                        <span class="text-base font-bold text-slate-800">${ch1.metal_loss.toFixed(2)} %</span>
                    </div>
                </div>

                <div class="flex gap-2">
                    <button onclick="triggerStation('${stID}')" class="flex-1 bg-blue-600 hover:bg-blue-700 text-white text-xs py-2 rounded font-medium transition">單點手動採集</button>
                    <button onclick="showHistory('${stID}')" class="bg-slate-100 hover:bg-slate-200 text-slate-700 text-xs px-3 py-2 rounded font-medium border transition">歷史紀錄</button>
                </div>
            </div>`;
            container.insertAdjacentHTML('beforeend', cardHtml);
        }
    } catch (e) {
        console.error('更新測站失敗:', e);
    }
}

async function triggerStation(stationID) {
    try {
        const res = await fetch(`/api/v1/trigger?station_id=${stationID}`, { method: 'POST' });
        if (!res.ok) throw new Error(await res.text());
        alert(`測站 [${stationID}] 手動採集成功！`);
        fetchStations();
    } catch (e) {
        alert(`採集失敗: ${e.message}`);
    }
}

async function updateInterval() {
    const sec = document.getElementById('pollIntervalSelect').value;
    const statusSpan = document.getElementById('schedulerStatus');

    await fetch('/api/v1/scheduler/interval', {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: `seconds=${sec}`
    });

    if (sec === "0") {
        statusSpan.innerText = "自動採集已停用";
        statusSpan.className = "inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-slate-100 text-slate-600";
    } else {
        statusSpan.innerText = "自動採集運作中";
        statusSpan.className = "inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800";
    }
}

async function showHistory(stationID) {
    document.getElementById('modalTitle').innerText = `[${stationID}] 歷史歸檔紀錄 (SQLite DB)`;
    const tbody = document.getElementById('historyTableBody');
    tbody.innerHTML = '<tr><td colspan="6" class="p-4 text-center">載入中...</td></tr>';
    document.getElementById('historyModal').classList.remove('hidden');

    const res = await fetch(`/api/v1/history?station_id=${stationID}&limit=30`);
    const history = await res.json();
    tbody.innerHTML = '';

    history.forEach(h => {
        const row = `<tr class="hover:bg-slate-50">
            <td class="p-2 font-mono">${h.timestamp}</td>
            <td class="p-2">${h.channel_1.thickness.toFixed(1)}</td>
            <td class="p-2">${h.channel_1.metal_loss.toFixed(2)} %</td>
            <td class="p-2">${(h.channel_1.e_off * 1000).toFixed(1)} mV</td>
            <td class="p-2">${h.channel_1.j_ac.toFixed(2)}</td>
            <td class="p-2">${h.channel_1.temperature.toFixed(1)} °C</td>
        </tr>`;
        tbody.insertAdjacentHTML('beforeend', row);
    });
}

function closeModal() {
    document.getElementById('historyModal').classList.add('hidden');
}

// 每 5 秒自動拉取 Dashobard 最新狀態
setInterval(fetchStations, 5000);
window.onload = fetchStations;