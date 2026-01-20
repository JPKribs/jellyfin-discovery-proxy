let refreshTime = Date.now();

// updateTimer updates the elapsed time display.
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

// refreshPage reloads the current page.
function refreshPage() {
    location.reload();
}

setInterval(updateTimer, 1000);
updateTimer();
