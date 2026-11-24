// lawgate site: the gate card in the hero, the demo, the example tabs, and copy buttons.
const root = document.documentElement;
root.classList.remove("no-js");
root.classList.add("js");

const reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

/* ---------- Gate card: check each law, pass one change, refuse the next ---------- */
// Two example changes to the wallet. The second deletes the balance check, so
// the two laws that depend on it break and the gate refuses the change.
const CHANGES = [
  { label: "Change 1: add a receipt email", broken: [] },
  { label: "Change 2: delete the balance check", broken: [0, 1] },
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
    const refused = current.broken.length > 0;
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
      const broken = current.broken.includes(i);
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

/* ---------- Demo: reveal the prompt and each outcome as it scrolls into view ---------- */
// Each card is watched on its own. The demo as a whole is taller than a phone
// screen, so a share of the whole demo might never be visible at once.
function demo() {
  const cards = document.querySelectorAll("#demo .demo-ask, #demo .outcome");
  const io = new IntersectionObserver((entries) => {
    entries.filter((entry) => entry.isIntersecting).forEach((entry) => {
      entry.target.classList.add("is-in");
      io.unobserve(entry.target);
    });
  }, { rootMargin: "0px 0px -10% 0px" });
  cards.forEach((card) => io.observe(card));
}

/* ---------- Example tabs: machine law, test law, kernel law ---------- */
function tabs() {
  const buttons = [...document.querySelectorAll('.tabs [role="tab"]')];
  const panels = [...document.querySelectorAll('.panel[role="tabpanel"]')];
  if (!buttons.length) return;
  const select = (name, focus) => {
    buttons.forEach((button) => {
      const on = button.dataset.tab === name;
      button.setAttribute("aria-selected", String(on));
      button.tabIndex = on ? 0 : -1;
      if (on && focus) button.focus();
    });
    panels.forEach((panel) => panel.setAttribute("aria-hidden", String(panel.dataset.tab !== name)));
  };
  buttons.forEach((button, i) => {
    button.addEventListener("click", () => select(button.dataset.tab));
    button.addEventListener("keydown", (event) => {
      const step = event.key === "ArrowRight" ? 1 : event.key === "ArrowLeft" ? -1 : 0;
      if (!step) return;
      const next = buttons[(i + step + buttons.length) % buttons.length];
      select(next.dataset.tab, true);
    });
  });
  select(buttons[0].dataset.tab);
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
demo();
tabs();
copyButtons();
