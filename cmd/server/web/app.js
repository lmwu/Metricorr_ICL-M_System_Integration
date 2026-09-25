// cmd/server/web/app.js

async function fetchStations() {
    try {
        const res = await fetch('/api/v1/stations');
        const data = await res.json();
        const container = document.getElementById('stationCards');
        container.innerHTML = '';

        if (!data || Object.keys(data).length === 0) {
            container.innerHTML = `<div class="col-span-full p-8 text-center bg-white rounded-xl border text-slate-400">目前尚無已連線之 4G 網關。請啟動 LTE 網關透傳腳本。</div>`;
            return;
        }

        for (const [stID, st] of Object.entries(data)) {
            const ch1 = st.channel_1 || {};
            const ch2 = st.channel_2 || {};

            // 判斷是否有啟用 Channel 2 (若有剩餘厚度或電位數據則視為啟用)
            const hasCh2 = ch2 && (typeof ch2.thickness === 'number' && ch2.thickness > 0 || typeof ch2.e_off === 'number' && ch2.e_off !== 0);

            // 狀態標籤
            const statusBadge = st.is_online
                ? `<span class="text-xs font-semibold text-emerald-600 bg-emerald-50 px-2 py-0.5 rounded border border-emerald-200">已連線</span>`
                : `<span class="text-xs font-semibold text-slate-400 bg-slate-100 px-2 py-0.5 rounded border border-slate-200">已離線</span>`;

            // Channel 1 數據解包
            const ch1_eOff = typeof ch1.e_off === 'number' ? (ch1.e_off * 1000).toFixed(1) : '--';
            const ch1_jAc = typeof ch1.j_ac === 'number' ? ch1.j_ac.toFixed(2) : '--';
            const ch1_thickness = typeof ch1.thickness === 'number' ? ch1.thickness.toFixed(1) : '--';
            const ch1_metalLoss = typeof ch1.metal_loss === 'number' ? ch1.metal_loss.toFixed(2) : '--';

            // Channel 2 數據解包
            const ch2_eOff = typeof ch2.e_off === 'number' ? (ch2.e_off * 1000).toFixed(1) : '--';
            const ch2_jAc = typeof ch2.j_ac === 'number' ? ch2.j_ac.toFixed(2) : '--';
            const ch2_thickness = typeof ch2.thickness === 'number' ? ch2.thickness.toFixed(1) : '--';
            const ch2_metalLoss = typeof ch2.metal_loss === 'number' ? ch2.metal_loss.toFixed(2) : '--';

            // 構建通道數據區域 HTML
            let channelsHtml = '';

            if (hasCh2) {
                // 雙通道（Ch1 & Ch2）並列渲染
                channelsHtml = `
                <div class="grid grid-cols-2 gap-4 mb-4 border-t pt-3">
                    <!-- Channel 1 區塊 -->
                    <div>
                        <div class="text-xs font-bold text-slate-700 mb-2 pb-1 border-b flex items-center gap-1">
                            <span class="w-2 h-2 rounded-full bg-blue-500"></span> Channel 1 (探頭 1)
                        </div>
                        <div class="grid grid-cols-2 gap-2 text-xs">
                            <div class="bg-slate-50 p-2 rounded">
                                <span class="text-slate-500 block text-[10px]">Eoff 斷電電位</span>
                                <span class="text-sm font-bold text-slate-800">${ch1_eOff} mV</span>
                            </div>
                            <div class="bg-slate-50 p-2 rounded">
                                <span class="text-slate-500 block text-[10px]">Jac AC 干擾</span>
                                <span class="text-sm font-bold ${ch1.j_ac > 30 ? 'text-red-600' : 'text-slate-800'}">${ch1_jAc} A/m²</span>
                            </div>
                            <div class="bg-slate-50 p-2 rounded">
                                <span class="text-slate-500 block text-[10px]">ER 剩餘厚度</span>
                                <span class="text-sm font-bold text-slate-800">${ch1_thickness} µm</span>
                            </div>
                            <div class="bg-slate-50 p-2 rounded">
                                <span class="text-slate-500 block text-[10px]">累積金屬損失</span>
                                <span class="text-sm font-bold text-slate-800">${ch1_metalLoss} %</span>
                            </div>
                        </div>
                    </div>

                    <!-- Channel 2 區塊 -->
                    <div>
                        <div class="text-xs font-bold text-slate-700 mb-2 pb-1 border-b flex items-center gap-1">
                            <span class="w-2 h-2 rounded-full bg-purple-500"></span> Channel 2 (探頭 2)
                        </div>
                        <div class="grid grid-cols-2 gap-2 text-xs">
                            <div class="bg-slate-50 p-2 rounded">
                                <span class="text-slate-500 block text-[10px]">Eoff 斷電電位</span>
                                <span class="text-sm font-bold text-slate-800">${ch2_eOff} mV</span>
                            </div>
                            <div class="bg-slate-50 p-2 rounded">
                                <span class="text-slate-500 block text-[10px]">Jac AC 干擾</span>
                                <span class="text-sm font-bold ${ch2.j_ac > 30 ? 'text-red-600' : 'text-slate-800'}">${ch2_jAc} A/m²</span>
                            </div>
                            <div class="bg-slate-50 p-2 rounded">
                                <span class="text-slate-500 block text-[10px]">ER 剩餘厚度</span>
                                <span class="text-sm font-bold text-slate-800">${ch2_thickness} µm</span>
                            </div>
                            <div class="bg-slate-50 p-2 rounded">
                                <span class="text-slate-500 block text-[10px]">累積金屬損失</span>
                                <span class="text-sm font-bold text-slate-800">${ch2_metalLoss} %</span>
                            </div>
                        </div>
                    </div>
                </div>`;
            } else {
                // 單通道（僅 Ch1）簡明渲染
                channelsHtml = `
                <div class="grid grid-cols-2 gap-3 mb-4 text-xs">
                    <div class="bg-slate-50 p-2.5 rounded">
                        <span class="text-slate-500 block">Ch1 斷電電位 Eoff</span>
                        <span class="text-base font-bold text-slate-800">${ch1_eOff} mV</span>
                    </div>
                    <div class="bg-slate-50 p-2.5 rounded">
                        <span class="text-slate-500 block">AC 干擾密度 Jac</span>
                        <span class="text-base font-bold ${ch1.j_ac > 30 ? 'text-red-600' : 'text-slate-800'}">${ch1_jAc} A/m²</span>
                    </div>
                    <div class="bg-slate-50 p-2.5 rounded">
                        <span class="text-slate-500 block">ER 探頭剩餘厚度</span>
                        <span class="text-base font-bold text-slate-800">${ch1_thickness} µm</span>
                    </div>
                    <div class="bg-slate-50 p-2.5 rounded">
                        <span class="text-slate-500 block">累積金屬損失</span>
                        <span class="text-base font-bold text-slate-800">${ch1_metalLoss} %</span>
                    </div>
                </div>`;
            }

            const cardHtml = `
            <div class="${hasCh2 ? 'col-span-2' : ''} bg-white p-5 rounded-xl border border-slate-200 shadow-sm hover:shadow-md transition">
                <div class="flex justify-between items-start pb-3 mb-3">
                    <div>
                        <span class="text-xs font-bold text-blue-600 bg-blue-50 px-2 py-0.5 rounded">測站 ID</span>
                        <h3 class="text-lg font-bold text-slate-800 mt-1">${stID}</h3>
                    </div>
                    <div class="text-right">
                        <div>${statusBadge}</div>
                        <div class="text-[10px] text-slate-400 mt-1">${st.timestamp || '尚未採集'}</div>
                    </div>
                </div>

                ${channelsHtml}

                <div class="flex gap-2 border-t pt-3">
                    <button onclick="triggerStation('${stID}')" class="flex-1 bg-blue-600 hover:bg-blue-700 text-white text-xs py-2 rounded font-medium transition">單點手動採集</button>
                    <button onclick="showHistory('${stID}')" class="bg-slate-100 hover:bg-slate-200 text-slate-700 text-xs px-3 py-2 rounded font-medium border transition">歷史紀錄</button>
                    <button onclick="deleteStation('${stID}')" class="bg-red-50 hover:bg-red-100 text-red-600 text-xs px-2.5 py-2 rounded font-medium border border-red-200 transition" title="廢止此測站">廢止</button>
                </div>
            </div>`;
            container.insertAdjacentHTML('beforeend', cardHtml);
        }
    } catch (e) {
        console.error('更新測站看板失敗:', e);
    }
}

async function deleteStation(stationID) {
    const confirmed = confirm(`確定要廢止並撤除測站 [${stationID}] 嗎？\n\n注意：此操作會將該卡片從動態看板移除，但過去所有歷史歸檔數據仍會完好保存於 SQLite 資料庫中。`);
    if (!confirmed) return;

    try {
        const res = await fetch(`/api/v1/station/delete?station_id=${stationID}`, { method: 'DELETE' });
        if (!res.ok) throw new Error(await res.text());
        
        alert(`測站 [${stationID}] 已成功廢止撤除！`);
        fetchStations();
    } catch (e) {
        alert(`廢止失敗: ${e.message}`);
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

    try {
        const res = await fetch(`/api/v1/history?station_id=${stationID}&limit=30`);
        const history = await res.json();
        tbody.innerHTML = '';

        if (!Array.isArray(history) || history.length === 0) {
            tbody.innerHTML = '<tr><td colspan="6" class="p-4 text-center text-slate-400">目前尚無歷史數據紀錄</td></tr>';
            return;
        }

        history.forEach(h => {
            const ch1 = h.channel_1 || {};
            const ch2 = h.channel_2 || {};
            const hasCh2 = ch2 && (ch2.thickness > 0 || ch2.e_off !== 0);

            const row = `<tr class="hover:bg-slate-50 border-b">
                <td class="p-2 font-mono text-[11px]">${h.timestamp}</td>
                <td class="p-2">
                    <div class="font-semibold text-slate-700">Ch1: ${(ch1.thickness ?? 0).toFixed(1)} µm</div>
                    ${hasCh2 ? `<div class="text-purple-600 text-[10px]">Ch2: ${(ch2.thickness ?? 0).toFixed(1)} µm</div>` : ''}
                </td>
                <td class="p-2">
                    <div>Ch1: ${(ch1.metal_loss ?? 0).toFixed(2)} %</div>
                    ${hasCh2 ? `<div class="text-purple-600 text-[10px]">Ch2: ${(ch2.metal_loss ?? 0).toFixed(2)} %</div>` : ''}
                </td>
                <td class="p-2">
                    <div>Ch1: ${((ch1.e_off ?? 0) * 1000).toFixed(1)} mV</div>
                    ${hasCh2 ? `<div class="text-purple-600 text-[10px]">Ch2: ${((ch2.e_off ?? 0) * 1000).toFixed(1)} mV</div>` : ''}
                </td>
                <td class="p-2">
                    <div>Ch1: ${(ch1.j_ac ?? 0).toFixed(2)} A/m²</div>
                    ${hasCh2 ? `<div class="text-purple-600 text-[10px]">Ch2: ${(ch2.j_ac ?? 0).toFixed(2)} A/m²</div>` : ''}
                </td>
                <td class="p-2">${(ch1.temperature ?? 0).toFixed(1)} °C</td>
            </tr>`;
            tbody.insertAdjacentHTML('beforeend', row);
        });
    } catch (e) {
        tbody.innerHTML = `<tr><td colspan="6" class="p-4 text-center text-red-500">載入歷史紀錄失敗: ${e.message}</td></tr>`;
    }
}

function closeModal() {
    document.getElementById('historyModal').classList.add('hidden');
}

setInterval(fetchStations, 5000);
window.onload = fetchStations;