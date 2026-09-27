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
    initSSE(); // 啟動即時事件監聽 (SSE)
});

// 啟用 SSE 即時監聽：網關連線/斷線時立即可見
function initSSE() {
    if (!window.EventSource) return;

    const eventSource = new EventSource(`${API_BASE}/stations/events`);

    eventSource.addEventListener("online_update", (e) => {
        try {
            const stationIDs = JSON.parse(e.data);
            updateStationDropdown(stationIDs);
        } catch (err) {
            console.error("解析 SSE 數據失敗:", err);
        }
    });

    eventSource.onerror = (err) => {
        console.warn("⚠️ SSE 連線中斷，嘗試自動重連中...");
    };
}

// 🌟【修改】：更新網關下拉選單與燈號指示邏輯
function updateStationDropdown(stationIDs) {
    const indicator = document.getElementById("statusIndicator");

    // 🔴 情況 A：無網關連線 (全數離線)
    if (!stationIDs || stationIDs.length === 0) {
        ui.stationSelect.innerHTML = '<option value="">無線上網關</option>';
        currentStation = "";
        ui.currentStationTitle.textContent = "網關離線";
        if (indicator) {
            indicator.className = "w-3.5 h-3.5 rounded-full bg-rose-500 shadow-lg shadow-rose-500/50 animate-pulse"; // 亮紅燈
        }
        return;
    }

    // 🟢 情況 B：至少有一個網關在線
    const previousSelected = currentStation;
    ui.stationSelect.innerHTML = "";

    stationIDs.forEach(id => {
        const opt = document.createElement("option");
        opt.value = id;
        opt.textContent = id + " (🟢 在線)";
        ui.stationSelect.appendChild(opt);
    });

    if (stationIDs.includes(previousSelected)) {
        ui.stationSelect.value = previousSelected;
    } else {
        currentStation = stationIDs[0];
        ui.stationSelect.value = currentStation;
        ui.currentStationTitle.textContent = currentStation;
        fetchLatestData();
        fetchHistoryData();
    }

    if (indicator) {
        indicator.className = "w-3.5 h-3.5 rounded-full bg-emerald-500 shadow-lg shadow-emerald-500/50 animate-pulse"; // 亮綠燈
    }
}

async function fetchStations() {
    try {
        const resOnline = await fetch(`${API_BASE}/stations/online`);
        const stationIDs = await resOnline.json();
        updateStationDropdown(stationIDs);
        scheduleNextPoll();
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
    ui.currentStationTitle.textContent = currentStation || "網關離線";
    fetchLatestData();
    fetchHistoryData();
}

async function fetchLatestData() {
    if (!currentStation) return;
    try {
        const res = await fetch(`${API_BASE}/stations/data/all`);
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

async function triggerMeasurement() {
    if (!currentStation) return;

    ui.btnTrigger.disabled = true;
    ui.btnTrigger.classList.add("opacity-50", "cursor-not-allowed");
    ui.btnText.textContent = "⏳ 探採進行中 (約需 30 秒)...";

    try {
        const res = await fetch(`${API_BASE}/stations/${currentStation}/measure`, {
            method: "POST",
            headers: {
                "Content-Type": "application/json"
            }
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

async function fetchHistoryData() {
    if (!currentStation || !chartInstance) return;

    try {
        const res = await fetch(`${API_BASE}/stations/${currentStation}/data/history`);
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