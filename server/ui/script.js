let store = { volunteers: [], schedule: [] };
let organizers = [];

const el = (id) => document.getElementById(id);

// Time helpers (backend stores local epoch seconds)
const fmtDT = (unix) => {
  const d = new Date(unix * 1000);
  const pad = (n) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(
    d.getDate(),
  )}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
};
const parseDT = (val) => Math.floor(new Date(val).getTime() / 1000);

// Generic helpers
async function api(path, opts = {}) {
  const r = await fetch(path, {
    headers: { "Content-Type": "application/json" },
    ...opts,
  });
  if (!r.ok) throw new Error(await r.text());
  return r.headers.get("content-type")?.includes("application/json")
    ? r.json()
    : r.text();
}

function msg(text, ok) {
  el("msg").textContent = text;
  el("msg").className = ok ? "ok" : "error";
}

// Organizers
async function loadOrganizers() {
  try {
    organizers = await api("/api/organizers");
    organizers.sort((a, b) => a.name.localeCompare(b.name));
  } catch (e) {
    organizers = [];
    console.error("Failed to load organizers:", e);
  }
}

// Rendering
function render() {
  const tbody = el("shift-rows");
  tbody.innerHTML = "";

  store.schedule.forEach((sh, i) => {
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td><input type="datetime-local" value="${fmtDT(
        sh.start,
      )}" data-k="start" /></td>
      <td><input type="datetime-local" value="${fmtDT(
        sh.end,
      )}" data-k="end" /></td>
      <td data-idx="${i}" class="user-cell"></td>
      <td>
        <button data-act="update">Save</button>
        <button data-act="delete" class="danger">Delete</button>
      </td>`;
    tbody.appendChild(tr);

    renderUserChips(tr.querySelector(".user-cell"), sh.user_ids || []);

    // Update
    tr.querySelector('[data-act="update"]').onclick = async () => {
      try {
        const start = parseDT(tr.querySelector('[data-k="start"]').value);
        const end = parseDT(tr.querySelector('[data-k="end"]').value);
        const users = store.schedule[i].user_ids || [];
        if (!(start > 0 && end > start))
          throw new Error("End must be after start.");
        await api(`/api/shift/${i}`, {
          method: "PUT",
          body: JSON.stringify({ start, end, user_ids: users }),
        });
        msg("Shift updated.", true);
        await load();
      } catch (e) {
        msg(e.message, false);
      }
    };

    // Delete
    tr.querySelector('[data-act="delete"]').onclick = async () => {
      if (!confirm("Delete this shift?")) return;
      try {
        await api(`/api/shift/${i}`, { method: "DELETE" });
        msg("Shift deleted.", true);
        await load();
      } catch (e) {
        msg(e.message, false);
      }
    };
  });

  // Volunteers
  const v = el("vol-list");
  v.innerHTML = "";
  store.volunteers.forEach((id) => {
    const chip = document.createElement("span");
    chip.className = "chip mono";
    chip.textContent = id + " ";
    const x = document.createElement("button");
    x.textContent = "×";
    x.onclick = async () => {
      try {
        await api(`/api/volunteers/${encodeURIComponent(id)}`, {
          method: "DELETE",
        });
        msg("Volunteer removed.", true);
        await load();
      } catch (e) {
        msg(e.message, false);
      }
    };
    chip.appendChild(x);
    v.appendChild(chip);
  });
}

// User chip editor with dropdown
function renderUserChips(cell, ids) {
  cell.innerHTML = "";
  const idx = +cell.dataset.idx;
  const wrap = document.createElement("div");
  wrap.className = "row flex-wrap";

  ids.forEach((uid) => {
    const chip = document.createElement("span");
    chip.className = "chip mono";
    const name = organizers.find((o) => o.id === uid)?.name || uid;
    chip.textContent = name + " ";
    const x = document.createElement("button");
    x.textContent = "×";
    x.onclick = () => {
      store.schedule[idx].user_ids = store.schedule[idx].user_ids.filter(
        (u) => u !== uid,
      );
      renderUserChips(cell, store.schedule[idx].user_ids);
    };
    chip.appendChild(x);
    wrap.appendChild(chip);
  });

  // Add dropdown
  const select = document.createElement("select");
  const placeholder = document.createElement("option");
  placeholder.textContent = "+ Add member";
  placeholder.value = "";
  select.appendChild(placeholder);

  organizers
    .filter((u) => !ids.includes(u.id))
    .forEach((u) => {
      const opt = document.createElement("option");
      opt.value = u.id;
      opt.textContent = u.name;
      select.appendChild(opt);
    });

  select.onchange = () => {
    const id = select.value;
    if (!id) return;
    const arr = store.schedule[idx].user_ids;
    if (!arr.includes(id)) {
      arr.push(id);
      renderUserChips(cell, arr);
    }
  };

  wrap.appendChild(select);
  cell.appendChild(wrap);
}

// Core actions
async function load() {
  const st = await api("/api/schedule");
  store = st;

  // Fetch organizers before rendering
  await loadOrganizers();

  const status = await api("/api/status").catch(() => null);
  if (status) {
    el("now").textContent = new Date(status.now * 1000).toLocaleString();
    el("cur").textContent = status.cur?.start
      ? `${status.cur.user_ids?.length || 0} active`
      : "(none)";
    el("next").textContent = status.next?.start
      ? `starts ${new Date(status.next.start * 1000).toLocaleTimeString()}`
      : "(none)";
  }
  render();
}

el("reload").onclick = load;

el("export").onclick = () => {
  const blob = new Blob([JSON.stringify(store, null, 2)], {
    type: "application/json",
  });
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob);
  a.download = "store.json";
  a.click();
};

el("save").onclick = async () => {
  try {
    const rows = [...document.querySelectorAll("#shift-rows tr")];
    const schedule = rows.map((tr, i) => {
      const start = parseDT(tr.querySelector('[data-k="start"]').value);
      const end = parseDT(tr.querySelector('[data-k="end"]').value);
      const users = store.schedule[i].user_ids || [];
      return { start, end, user_ids: users };
    });
    for (const s of schedule) {
      if (!(s.start > 0 && s.end > s.start))
        throw new Error("All shifts must have end>start.");
    }
    const body = { volunteers: store.volunteers, schedule };
    await api("/api/schedule", { method: "POST", body: JSON.stringify(body) });
    msg("All changes saved.", true);
    await load();
  } catch (e) {
    msg(e.message, false);
  }
};

el("add-shift").onclick = async () => {
  const now = Date.now();
  const s = Math.floor(now / 1000) + 3600;
  const e = s + 2 * 3600;
  try {
    await api("/api/shift", {
      method: "POST",
      body: JSON.stringify({ start: s, end: e, user_ids: [] }),
    });
    await load();
    msg("Shift added.", true);
  } catch (e) {
    msg(e.message, false);
  }
};

el("add-vol").onclick = async () => {
  const id = el("vol-id").value.trim();
  if (!id) return;
  try {
    await api("/api/volunteers", {
      method: "POST",
      body: JSON.stringify({ user_id: id }),
    });
    el("vol-id").value = "";
    await load();
    msg("Volunteer added.", true);
  } catch (e) {
    msg(e.message, false);
  }
};

load();
