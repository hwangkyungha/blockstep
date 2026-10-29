// 필터 콤보. data-combo="people" 같은 입력 칸에 초점이 가거나 글자를 치면 칸 아래에 목록을 열고,
// 친 글자가 포함된 항목만 남긴다(대소문자 무시). 방향키로 이동, Enter로 선택, Esc나 칸 밖 클릭으로 닫는다.
// 항목은 <script type="application/json" id="combo-<이름>"> 안의 JSON 배열이다. 목록에 없는 값도 그대로 입력된다.
(function () {
  var sources = {};
  function options(name) {
    if (!(name in sources)) {
      var el = document.getElementById('combo-' + name);
      sources[name] = el ? JSON.parse(el.textContent) : [];
    }
    return sources[name];
  }

  var box = document.createElement('div');
  box.className = 'combo-list hidden';
  box.setAttribute('role', 'listbox');
  document.body.appendChild(box);
  var current = null;   // 열려 있는 입력 칸
  var active = -1;      // 방향키로 고른 항목 번호
  var suppress = false; // 선택 뒤 초점을 되돌릴 때 다시 열리지 않게 한다

  function close() {
    box.classList.add('hidden');
    box.innerHTML = '';
    current = null;
    active = -1;
  }

  function open(input) {
    var q = input.value.trim().toLowerCase();
    var items = options(input.dataset.combo).filter(function (v) { return v.toLowerCase().indexOf(q) !== -1; });
    if (!items.length) { close(); return; }
    current = input;
    active = -1;
    box.innerHTML = '';
    items.forEach(function (v) {
      var it = document.createElement('div');
      it.className = 'combo-item';
      it.setAttribute('role', 'option');
      it.textContent = v;
      it.addEventListener('mousedown', function (e) { e.preventDefault(); pick(v); });
      box.appendChild(it);
    });
    var r = input.getBoundingClientRect();
    box.style.left = (r.left + window.scrollX) + 'px';
    box.style.top = (r.bottom + window.scrollY + 2) + 'px';
    box.style.minWidth = r.width + 'px';
    box.classList.remove('hidden');
  }

  function pick(v) {
    if (!current) { return; }
    current.value = v;
    current.dispatchEvent(new Event('input', { bubbles: true }));
    var input = current;
    close();
    suppress = true;
    input.focus();
    suppress = false;
  }

  function move(delta) {
    var items = box.querySelectorAll('.combo-item');
    if (!items.length) { return; }
    if (active >= 0) { items[active].classList.remove('active'); }
    active = (active + delta + items.length) % items.length;
    items[active].classList.add('active');
    items[active].scrollIntoView({ block: 'nearest' });
  }

  document.addEventListener('focusin', function (e) {
    if (!suppress && e.target.dataset && e.target.dataset.combo) { open(e.target); }
  });
  document.addEventListener('input', function (e) {
    if (e.target.dataset && e.target.dataset.combo && document.activeElement === e.target) { open(e.target); }
  });
  document.addEventListener('keydown', function (e) {
    if (!current || e.target !== current) { return; }
    if (e.key === 'ArrowDown') { e.preventDefault(); move(1); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); move(-1); }
    else if (e.key === 'Enter') {
      if (active >= 0) { e.preventDefault(); pick(box.querySelectorAll('.combo-item')[active].textContent); }
      else { close(); }
    }
    else if (e.key === 'Escape') { e.preventDefault(); close(); }
    else if (e.key === 'Tab') { close(); }
  });
  document.addEventListener('mousedown', function (e) {
    if (current && e.target !== current && !box.contains(e.target)) { close(); }
  });
})();
