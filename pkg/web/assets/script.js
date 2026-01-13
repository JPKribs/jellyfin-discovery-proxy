let refreshTime = Date.now();

function updateTimer() {
    const elapsed = Math.floor((Date.now() - refreshTime) / 1000);
    const minutes = Math.floor(elapsed / 60);
    const seconds = elapsed % 60;

    let timeStr = '';
    if (minutes > 0) {
        timeStr = minutes + 'm ' + seconds + 's';
    } else {
        timeStr = seconds + 's';
    }

    document.getElementById('refresh-timer').textContent = 'Since last refresh: ' + timeStr;
}

function refreshPage() {
    location.reload();
}

// Update timer every second
setInterval(updateTimer, 1000);
updateTimer();
