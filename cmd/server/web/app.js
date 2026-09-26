// 自動偵測當前 Server API Host，匹配 router.go 的路由
const API_BASE = `${window.location.protocol}//${window.location.host}/api`;

let currentStation = "";
let chartInstance = null;
let pollTimeout = null;

const ui = {
    stationSelect: null,
    currentStationTitle: null,
    lastUpdated: null,
    btnTrigger: null,
    btnText: null,
    ch1: {},
    ch2: {}
};

document.addEventListener("DOMContentLoaded", () => {
    ui.stationSelect = document.getElementById("stationSelect");
    ui.currentStationTitle = document.getElementById("currentStationTitle");
    ui.lastUpdated = document.getElementById("lastUpdated");
    ui.btnTrigger = document.getElementById("btnTrigger");
    ui.btnText = document.getElementById("btnText");

    ['eoff', 'jac', 'thickness', 'metal_loss', 'eon', 'uac', 'temp'].forEach(key => {
        ui.ch1[key] = document.getElementById(`ch1_${key}`);
        ui.ch2[key] = document.getElementById(`ch2_${key}`);
    });

    initChart();
    fetchStations();
});

// 1. 取得所有測站清單並填入下拉選單 (對應 /api/data/all)
async function fetchStations() {
    try {
        const res = await fetch(`${API_BASE}/data/all`);
        const data = await res.json();
        
        if (data) {
            ui.stationSelect.innerHTML = "";
            const stationIDs = Object.keys(data);
            if (stationIDs.length === 0) {
                ui.stationSelect.innerHTML = '<option value="">無可用測站</option>';
                return;
            }

            stationIDs.forEach(id => {
                const opt = document.createElement("option");
                opt.value = id;
                opt.textContent = id;
                ui.stationSelect.appendChild(opt);
            });

            currentStation = stationIDs[0];
            ui.currentStationTitle.textContent = currentStation;
            
            updateDashboardUI(data[currentStation]);
            fetchHistoryData();
            scheduleNextPoll();
        }
    } catch (err) {
        console.error("❌ 無法取得測站列表:", err);
    }
}

function scheduleNextPoll() {
    if (pollTimeout) clearTimeout(pollTimeout);
    pollTimeout = setTimeout(async () => {
        if (currentStation) {
            await fetchLatestData();
            await fetchHistoryData();
        }
        scheduleNextPoll();
    }, 10000);
}

function switchStation() {
    currentStation = ui.stationSelect.value;
    ui.currentStationTitle.textContent = currentStation;
    fetchLatestData();
    fetchHistoryData();
}

// 3. 取得最新數據 (對應 /api/data/all)
async function fetchLatestData() {
    try {
        const res = await fetch(`${API_BASE}/data/all`);
        const data = await res.json();
        if (data && data[currentStation]) {
            updateDashboardUI(data[currentStation]);
        }
    } catch (err) {
        console.error("❌ 刷新最新數據失敗:", err);
    }
}

function formatVal(val, decimals = 2) {
    return (val !== null && val !== undefined && !isNaN(val)) ? Number(val).toFixed(decimals) : "--";
}

// 4. 更新前端 UI
function updateDashboardUI(channels) {
    if (!channels) return;

    if (channels[1]) {
        const d = channels[1];
        ui.ch1.eoff.textContent = formatVal(d.e_off, 3);
        ui.ch1.jac.textContent = formatVal(d.j_ac, 2);
        ui.ch1.thickness.textContent = formatVal(d.thickness, 1);
        ui.ch1.metal_loss.textContent = formatVal(d.metal_loss, 2);
        ui.ch1.eon.textContent = formatVal(d.e_on, 3);
        ui.ch1.uac.textContent = formatVal(d.uac, 2);
        ui.ch1.temp.textContent = formatVal(d.temperature, 1);
    }

    if (channels[2]) {
        const d = channels[2];
        ui.ch2.eoff.textContent = formatVal(d.e_off, 3);
        ui.ch2.jac.textContent = formatVal(d.j_ac, 2);
        ui.ch2.thickness.textContent = formatVal(d.thickness, 1);
        ui.ch2.metal_loss.textContent = formatVal(d.metal_loss, 2);
        ui.ch2.eon.textContent = formatVal(d.e_on, 3);
        ui.ch2.uac.textContent = formatVal(d.uac, 2);
        ui.ch2.temp.textContent = formatVal(d.temperature, 1);
    }

    ui.lastUpdated.textContent = new Date().toLocaleTimeString();
}

// 5. 觸發手動探採工作流 (對應 /api/measure)
async function triggerMeasurement() {
    if (!currentStation) return;

    ui.btnTrigger.disabled = true;
    ui.btnTrigger.classList.add("opacity-50", "cursor-not-allowed");
    ui.btnText.textContent = "⏳ 探採進行中 (約需 30 秒)...";

    try {
        const res = await fetch(`${API_BASE}/measure`, {
            method: "POST",
            headers: {
                "Content-Type": "application/json"
            },
            body: JSON.stringify({ station_id: currentStation })
        });
        
        if (res.ok) {
            let pollCount = 0;
            const pollStatus = async () => {
                pollCount++;
                await fetchLatestData();
                await fetchHistoryData();

                if (pollCount >= 8) { 
                    ui.btnTrigger.disabled = false;
                    ui.btnTrigger.classList.remove("opacity-50", "cursor-not-allowed");
                    ui.btnText.textContent = "⚡ 立即手動探採";
                } else {
                    setTimeout(pollStatus, 5000);
                }
            };
            setTimeout(pollStatus, 5000);
        } else {
            const errJson = await res.json();
            alert(`⚠️ ${errJson.error || "觸發失敗"}`);
            ui.btnTrigger.disabled = false;
            ui.btnTrigger.classList.remove("opacity-50", "cursor-not-allowed");
            ui.btnText.textContent = "⚡ 立即手動探採";
        }
    } catch (err) {
        alert("❌ 網絡連線異常！");
        ui.btnTrigger.disabled = false;
        ui.btnTrigger.classList.remove("opacity-50", "cursor-not-allowed");
        ui.btnText.textContent = "⚡ 立即手動探採";
    }
}

function initChart() {
    const ctx = document.getElementById("trendChart").getContext("2d");
    chartInstance = new Chart(ctx, {
        type: 'line',
        data: {
            labels: [],
            datasets: [
                {
                    label: 'Ch1 Eoff (V)',
                    data: [],
                    borderColor: '#34d399',
                    backgroundColor: 'rgba(52, 211, 153, 0.1)',
                    yAxisID: 'yEoff',
                    tension: 0.3
                },
                {
                    label: 'Ch1 剩餘厚度 (µm)',
                    data: [],
                    borderColor: '#38bdf8',
                    backgroundColor: 'rgba(56, 189, 248, 0.1)',
                    yAxisID: 'yThickness',
                    tension: 0.3
                }
            ]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            scales: {
                x: { grid: { color: 'rgba(255, 255, 255, 0.05)' }, ticks: { color: '#94a3b8' } },
                yEoff: { type: 'linear', position: 'left', title: { display: true, text: 'Eoff 電位 (V)', color: '#34d399' }, grid: { color: 'rgba(255, 255, 255, 0.05)' }, ticks: { color: '#34d399' } },
                yThickness: { type: 'linear', position: 'right', title: { display: true, text: '厚度 (µm)', color: '#38bdf8' }, grid: { drawOnChartArea: false }, ticks: { color: '#38bdf8' } }
            },
            plugins: { legend: { labels: { color: '#f8fafc' } } }
        }
    });
}

// 7. 撈取歷史紀錄並繪圖 (對應 /api/data/history?station=XXX)
async function fetchHistoryData() {
    if (!currentStation || !chartInstance) return;

    try {
        const res = await fetch(`${API_BASE}/data/history?station=${currentStation}`);
        const data = await res.json();

        if (Array.isArray(data)) {
            const ch1Logs = data.filter(l => l.channel === 1).reverse();
            chartInstance.data.labels = ch1Logs.map(l => new Date(l.created_at).toLocaleTimeString());
            chartInstance.data.datasets[0].data = ch1Logs.map(l => l.e_off);
            chartInstance.data.datasets[1].data = ch1Logs.map(l => l.thickness);
            chartInstance.update();
        }
    } catch (err) {
        console.error("❌ 獲取歷史數據失敗:", err);
    }
}