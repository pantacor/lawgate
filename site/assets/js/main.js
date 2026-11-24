// lawgate site: the gate card demo in the hero, and copy buttons.
const root = document.documentElement;
root.classList.remove("no-js");
root.classList.add("js");

const reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

/* ---------- Gate card: check each law, pass one change, refuse the next ---------- */
// Two example changes. The second removes the payload check, so the law that
// forbids an empty payload breaks and the gate refuses the change.
const CHANGES = [
  { label: "Change 1: add a retry log line", broken: -1 },
  { label: "Change 2: remove the payload check", broken: 3 },
];

function gateCard() {
  const list = document.getElementById("gate-laws");
  if (!list || reduceMotion) return;
  const phase = document.getElementById("gate-phase");
  const change = document.getElementById("gate-change");
  const rows = [...list.children];
  const stepMs = 520;
  const restMs = 3200;

  let timer = null;
  let turn = 0;

  function setRow(row, state, text) {
    row.classList.remove("is-waiting", "is-broken");
    if (state) row.classList.add(state);
    row.querySelector(".verdict").textContent = text;
  }

  function setPhase(text, refused) {
    phase.textContent = text;
    phase.classList.toggle("is-refused", refused);
  }

  function settle() {
    rows.forEach((row) => setRow(row, "", "Holds"));
    setPhase("Passed", false);
    change.textContent = `${rows.length} laws hold`;
  }

  function finish(current) {
    const refused = current.broken >= 0;
    setPhase(refused ? "Refused" : "Passed", refused);
    timer = setTimeout(check, restMs);
  }

  function check() {
    const current = CHANGES[turn % CHANGES.length];
    turn++;
    rows.forEach((row) => setRow(row, "is-waiting", "Waiting"));
    setPhase("Checking", false);
    change.textContent = current.label;
    let i = 0;
    timer = setInterval(() => {
      const broken = i === current.broken;
      setRow(rows[i], broken ? "is-broken" : "", broken ? "Broken" : "Holds");
      i++;
      if (i < rows.length) return;
      clearInterval(timer);
      finish(current);
    }, stepMs);
  }

  // Start once the card is on screen; settle when it scrolls away.
  let running = false;
  const io = new IntersectionObserver(([entry]) => {
    if (entry.isIntersecting === running) return;
    running = entry.isIntersecting;
    clearInterval(timer);
    clearTimeout(timer);
    if (running) timer = setTimeout(check, 900);
    else settle();
  }, { threshold: 0.4 });
  io.observe(list);
}

/* ---------- Copy buttons ---------- */
function copyButtons() {
  document.querySelectorAll("[data-copy]").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const code = document.getElementById(btn.dataset.copy);
      try {
        await navigator.clipboard.writeText(code.textContent);
        btn.textContent = "Copied";
      } catch {
        const r = document.createRange();
        r.selectNodeContents(code);
        const s = getSelection();
        s.removeAllRanges();
        s.addRange(r);
        btn.textContent = "Selected";
      }
      setTimeout(() => { btn.textContent = "Copy"; }, 1600);
    });
  });
}

gateCard();
copyButtons();
