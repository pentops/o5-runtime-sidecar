function doRequest(method, path, body) {
  return new Promise((resolve, reject) => {
    let xhr = new XMLHttpRequest();
    xhr.open(method, path);
    xhr.onload = () => {
      response = JSON.parse(xhr.responseText);
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve(response);
      } else {
        reject({status: xhr.status, body: response});
      }
    }
    xhr.onerror = () => reject(xhr.statusText);
    if (body) {
      xhr.setRequestHeader("Content-Type", "application/json");
      xhr.send(JSON.stringify(body));
    } else {
      xhr.send();
    }
  });
}

async function login(e) {
  e.preventDefault();
  let email = document.getElementById('email').value;
  let password = document.getElementById('password').value;
  const resp = await doRequest("POST", "/portal/v1/session/login", {"basic": {email, password}});
  console.log("Login Response", resp);
  whoami();
}

async function logout() {
  var resp
  try {
    resp = await doRequest("POST", "/portal/v1/session/logout");
  } catch (e) {
    console.log("Logout Error", e);
    showMessage(e)
    return
  }

  console.log("Logout Response", resp);
  whoami();
}

async function whoami() {
  document.getElementById("message").innerHTML = "Loading...";
  const r2 = await doRequest("GET", "/portal/v1/session/whoami");
  console.log("Whoami Response", r2);
  showMessage(r2);
}

function showMessage(msg) {
  document.getElementById("message").innerHTML = JSON.stringify(msg, null, "  ");
}

document.getElementById("loginForm").addEventListener('submit', login);
document.getElementById("whoami").addEventListener('click', whoami);
document.getElementById("logout").addEventListener('click', logout);

whoami();
