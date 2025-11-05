async function load() {
  const res = await fetch("/api/schedule");
  document.getElementById("data").value = await res.text();
}
document.getElementById("save").onclick = async () => {
  const text = document.getElementById("data").value;
  const res = await fetch("/api/schedule", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: text,
  });
  alert(await res.text());
};
load();
