
function setURL(val) {
    document.getElementById("url").value = window.location.protocol + "//" + window.location.host + val
    return true;
}