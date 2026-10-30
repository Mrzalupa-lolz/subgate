/* SubGate operations console. No third-party scripts or persistent auth tokens. */
(() => {
  'use strict';

  const $ = (selector, root = document) => root.querySelector(selector);
  const app = $('#app');
  const modal = $('#modal');
  const modalForm = $('#modal-form');
  const modalContent = $('#modal-content');
  const number = new Intl.NumberFormat('ru-RU');
  const state = {
    user: null, status: null, route: 'overview', data: {}, filters: {},
    loading: false, controller: null, poll: null, stream: null, revision: 0,
    toastTimer: null, searchTimer: null, chartCleanup: null, updated: null, dialogRevision: 0, configEditor: null,
  };

  const paths = {
    injections: '<path d="M12 5v14M5 12h14"/>',
    overview: '<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/>',
    live: '<path d="M3 12h4l3-8 4 16 3-8h4"/>',
    nodes: '<rect x="3" y="3" width="18" height="7" rx="2"/><rect x="3" y="14" width="18" height="7" rx="2"/><path d="M7 6.5h.01M7 17.5h.01M16 6.5h2M16 17.5h2"/>',
    users: '<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2M22 21v-2a4 4 0 0 0-3-3.87"/><circle cx="9" cy="7" r="4"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/>',
    agents: '<path d="m12 3 8 4v10l-8 4-8-4V7l8-4ZM4 7l8 4 8-4M12 11v10"/>',
    tiers: '<path d="m12 3 9 5-9 5-9-5 9-5ZM3 12l9 5 9-5M3 16l9 5 9-5"/>',
    policies: '<path d="m12 3 8 3v6c0 5-8 9-8 9s-8-4-8-9V6l8-3Z"/><path d="m8 12 3 3 5-6"/>',
    pins: '<path d="m16 3 5 5-5 1-4 4-1 5-5-5 5-1 4-4 1-5ZM6 18l-3 3"/>',
    'geo-rules': '<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3a18 18 0 0 1 0 18 18 18 0 0 1 0-18Z"/>',
    activity: '<path d="M8 3H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-3M8 7h6M7 12h6M7 17h4M16 3h5v5M21 3l-8 8"/>',
    ipintel: '<circle cx="10" cy="10" r="7"/><path d="m15 15 6 6M10 7v6M7 10h6"/>',
    alerts: '<path d="M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9M10 21h4"/>',
    backups: '<path d="M4 4h16v5H4zM5 9v11h14V9M9 13h6"/>',
    audit: '<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8l-6-6ZM14 2v6h6M8 13h8M8 17h6"/>',
    settings: '<path d="M4 6h16M4 12h16M4 18h16"/><circle cx="8" cy="6" r="2"/><circle cx="16" cy="12" r="2"/><circle cx="10" cy="18" r="2"/>',
    refresh: '<path d="M21 3v6h-6M3 21v-6h6M3 9a9 9 0 0 1 15-5l3 5M21 15a9 9 0 0 1-15 5l-3-5"/>',
    arrow: '<path d="M5 12h14m-5-5 5 5-5 5"/>',
    plus: '<path d="M12 5v14M5 12h14"/>',
    download: '<path d="M12 3v12m-5-5 5 5 5-5M4 16v5h16v-5"/>',
    menu: '<path d="M4 6h16M4 12h16M4 18h16"/>',
    logout: '<path d="M9 3H4v18h5M9 12h12m-4-4 4 4-4 4"/>',
    check: '<path d="m5 12 4 4L19 6"/>',
    clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
    error: '<path d="m12 3 10 18H2L12 3ZM12 9v4M12 17h.01"/>',
  };
  const pages = {
    injections: ['Инъекции', 'Общие дополнительные профили для подписок Xray JSON.'],
    overview: ['Обзор', 'Вся инфраструктура в одном месте. Состояние флота, шлюза и телеметрии.'],
    live: ['Live-трафик', 'Текущая скорость по пользователям и нодам. Только свежие измерения агентов.'],
    nodes: ['Ноды', 'Состояние и протоколы нод, синхронизированных с Remnawave.'],
    users: ['Пользователи', 'Управление доступом, сроком подписки и счётчиками трафика.'],
    agents: ['Агенты', 'Подключение коллекторов Xray, состояние сбора и диагностика.'],
    tiers: ['Тарифные группы', 'Группы нод, стратегия выбора и резервная группа.'],
    policies: ['Политики', 'Реестр политик доступа с привязкой к тарифной группе.'],
    pins: ['Закрепления', 'Реестр индивидуальных привязок пользователя к ноде.'],
    'geo-rules': ['Геоправила', 'Проверка адреса клиента при запросе подписки.'],
    activity: ['Журнал активности', 'Назначения соединений из журналов Xray. Доступ и экспорт фиксируются в аудите.'],
    ipintel: ['IP Intelligence', 'Проверка IPv4 и IPv6 по локальным источникам IP-данных.'],
    alerts: ['Оповещения', 'Реестр событий и заметок для оператора.'],
    backups: ['Резервные копии', 'Зашифрованные согласованные снимки базы данных SubGate.'],
    audit: ['Аудит', 'История входов, изменений конфигурации и доступа к чувствительным данным.'],
    settings: ['Настройки', 'Фактическая конфигурация запущенного Core и состояние компонентов.'],
  };
  const schemas = {
    injections: { help: 'Старые правила сохраняют прежний режим. Полный профиль добавляется после маршрутизации во все совместимые подписки /v2ray-json.', sample: {} },
    tiers: {
      sample: { name: 'Europe', strategy: 'leastLoad', min_healthy: 1, node_ids: [], fallback_tier_id: '', enabled: true },
      help: 'name — обязательное имя. strategy: random, roundRobin, leastPing или leastLoad. node_ids — UUID нод; min_healthy — минимальное число онлайн-нод; fallback_tier_id — ID резервной группы без циклов; enabled включает группу. leastLoad использует число пользователей. leastPing выбирает только успешную TCP/TLS пробу не старше 10 минут; если проб нет, используется резерв или исходная подписка. Политика выбирает ноду из группы и фильтрует распознанные адреса подписки. Неизвестные форматы и адреса сохраняются.',
    },
    policies: {
      sample: { name: 'Default', tier_id: '', conditions: { countries: [], asns: [], user_agent: '' }, priority: 10, enabled: true },
      help: 'name — обязательное имя. tier_id ссылается на существующую группу. conditions: countries — коды стран, asns — ASN строками без AS, user_agent — часть строки клиента. Пустые условия подходят всем клиентам. priority — порядок проверки (меньше раньше); enabled включает политику. Первая подходящая политика выбирает ноду из группы для распознанных адресов подписки. Неизвестные протоколы и неоднозначные адреса сохраняются.',
    },
    pins: {
      sample: { user_id: '', node_id: '', enabled: true },
      help: 'user_id и node_id обязательны и должны существовать в синхронизированном реестре. enabled включает закрепление. Онлайн-нода пользователя выбирается раньше тарифной политики. Core фильтрует только распознанные адреса подписки; неизвестные протоколы и неоднозначные адреса сохраняются. Если нода недоступна, проверяются политики.',
    },
    'geo-rules': {
      sample: { name: 'Blocked network', rule_type: 'cidr', action: 'deny', values: ['192.0.2.0/24'], enabled: true },
      help: 'name — обязательное имя. rule_type: country, asn, cidr или network_type. action: allow или deny. values — непустой массив строк: коды стран (FI), ASN без AS (12345), CIDR или тип сети (hosting). enabled включает проверку. Правила применяются к запросам подписок; первое совпадение определяет решение. Для country нужны IP-данные с достаточной уверенностью.',
    },
    alerts: {
      sample: { name: 'Заметка оператора', severity: 'info', message: '', node_id: '', created_at: new Date().toISOString() },
      help: 'name, message, severity (info, warning, critical), node_id и created_at описывают событие. Это реестр заметок; автоматическая отправка уведомлений и вычисление условий в этой версии не подключены.',
    },
  };

  function esc(value) {
    return String(value ?? '').replace(/[&<>"']/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[char]);
  }
  function icon(name) { return `<svg class="icon" viewBox="0 0 24 24" aria-hidden="true">${paths[name] || paths.overview}</svg>`; }
  function finite(value) { const n = Number(value); return Number.isFinite(n) ? n : 0; }
  function count(value) { return value == null ? '—' : number.format(finite(value)); }
  function bytes(value) {
    if (value == null) return '—';
    let n = Math.max(0, finite(value));
    const units = ['Б', 'КиБ', 'МиБ', 'ГиБ', 'ТиБ'];
    let i = 0;
    while (n >= 1024 && i < units.length - 1) { n /= 1024; i++; }
    return `${number.format(Number(n.toFixed(i ? 1 : 0)))} ${units[i]}`;
  }
  function speed(value) {
    if (value == null) return '—';
    let n = Math.max(0, finite(value));
    const units = ['бит/с', 'Кбит/с', 'Мбит/с', 'Гбит/с', 'Тбит/с'];
    let i = 0;
    while (n >= 1000 && i < units.length - 1) { n /= 1000; i++; }
    return `${number.format(Number(n.toFixed(i ? 2 : 0)))} ${units[i]}`;
  }
  function parseDate(value) {
    if (!value || String(value).startsWith('0001-')) return null;
    let normalized = value;
    if (typeof value === 'string' && /^\d{4}-\d\d-\d\d \d\d:\d\d:\d\d$/.test(value)) normalized = value.replace(' ', 'T') + 'Z';
    const date = new Date(normalized);
    return Number.isNaN(date.getTime()) ? null : date;
  }
  function date(value, compact = false) {
    const parsed = parseDate(value);
    return parsed ? parsed.toLocaleString('ru-RU', compact ? { hour: '2-digit', minute: '2-digit', second: '2-digit' } : { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' }) : '—';
  }
  function duration(seconds) {
    const n = Math.max(0, finite(seconds));
    if (n >= 86400) return `${Math.floor(n / 86400)} д ${Math.floor(n % 86400 / 3600)} ч`;
    if (n >= 3600) return `${Math.floor(n / 3600)} ч ${Math.floor(n % 3600 / 60)} мин`;
    return `${Math.floor(n / 60)} мин`;
  }
  function canWrite() { return Boolean(state.user); }
  function principal(value) { return { ID: value.id ?? value.ID, Username: String(value.username ?? value.Username ?? '') }; }
  function badge(status, online) {
    const key = String(status || 'unknown').toLowerCase();
    const labels = { online: 'Онлайн', active: 'Активен', offline: 'Офлайн', disabled: 'Отключён', expired: 'Истёк', limited: 'Лимит', error: 'Ошибка', revoked: 'Отозван', unknown: 'Нет данных', warning: 'Внимание', critical: 'Критично', info: 'Информация', success: 'Успешно', failure: 'Ошибка', accepted: 'Разрешено', rejected: 'Отклонено' };
    const healthy = !['error', 'critical', 'failure', 'rejected'].includes(key) && (online === true || ['online', 'active', 'success', 'accepted'].includes(key));
    const tone = healthy ? '' : ['error', 'critical', 'failure'].includes(key) ? 'red' : ['warning', 'limited', 'expired'].includes(key) ? 'amber' : 'neutral';
    return `<span class="badge ${tone}">${esc(labels[key] || status || 'Нет данных')}</span>`;
  }
  function empty(title, detail, name = state.route, action = '') {
    return `<div class="empty">${icon(name)}<strong>${esc(title)}</strong><p>${esc(detail)}</p>${action}</div>`;
  }
  function table(headers, rows, options = {}) {
    if (!rows.length) return empty(options.title || 'Пока нет данных', options.detail || 'Данные появятся после подключения и синхронизации.', options.icon);
    return `<div class="table-wrap"><table><thead><tr>${headers.map(item => `<th scope="col">${esc(item)}</th>`).join('')}</tr></thead><tbody>${rows.map(row => `<tr>${row.map(cell => `<td>${cell}</td>`).join('')}</tr>`).join('')}</tbody></table></div>`;
  }
  function panel(title, body, aside = '') { return `<section class="panel"><div class="panel-head"><h2>${esc(title)}</h2>${aside}</div>${body}</section>`; }
  function metric(title, value, foot, glyph, suffix = '') {
    return `<article class="metric"><div class="metric-head">${esc(title)}${icon(glyph)}</div><div class="metric-value">${esc(value)}${suffix ? `<small>${esc(suffix)}</small>` : ''}</div><div class="metric-foot">${esc(foot)}</div></article>`;
  }
  function filters() { return state.filters[state.route] || (state.filters[state.route] = { q: '', offset: 0 }); }
  function actionButton(action, text, extras = '', classes = '') {
    return `<button type="button" class="${classes}" data-action="${action}" ${extras}>${esc(text)}</button>`;
  }
  function refreshButton() { return `<button type="button" data-action="refresh" aria-label="Обновить данные">${icon('refresh')}Обновить</button>`; }
  function pageHead(actions = '') {
    const [title, description] = pages[state.route];
    return `<header class="page-head"><div><div class="eyebrow">Operations / ${esc(state.route)}</div><h1>${esc(title)}</h1><p>${esc(description)}</p></div><div class="head-actions">${actions}${refreshButton()}</div></header>`;
  }
  function toolbar(placeholder) {
    return `<div class="toolbar"><label class="search-field"><span class="sr-only">${esc(placeholder)}</span><input id="page-search" type="search" autocomplete="off" data-filter="q" value="${esc(filters().q)}" placeholder="${esc(placeholder)}"></label><small>${state.loading ? 'Обновление…' : 'Обновляется автоматически'}</small></div>`;
  }
  function pageFoot(total, loaded) {
    const offset = finite(filters().offset);
    return `<div class="table-foot"><span>${total ? `${count(offset + 1)}–${count(offset + loaded)} из ${count(total)}` : '0 записей'}</span><div class="pager">${actionButton('previous', 'Назад', offset === 0 ? 'disabled' : '')}${actionButton('next', 'Далее', offset + loaded >= total ? 'disabled' : '')}</div></div>`;
  }

  class APIError extends Error {
    constructor(message, status) { super(message); this.name = 'APIError'; this.status = status; }
  }
  async function request(path, options = {}) {
    const method = options.method || 'GET';
    const headers = { Accept: 'application/json' };
    if (method !== 'GET' && method !== 'HEAD') headers['X-SubGate-CSRF'] = '1';
    if (options.body !== undefined) headers['Content-Type'] = 'application/json';
    let response;
    try {
      response = await fetch(`/api/v1${path}`, {
        credentials: 'same-origin', cache: 'no-store', ...options, method, headers,
        body: options.body === undefined ? undefined : JSON.stringify(options.body),
      });
    } catch (error) {
      if (error.name === 'AbortError') throw error;
      throw new APIError('Не удалось связаться с Core. Проверьте соединение.', 0);
    }
    if (!response.ok) {
      const payload = await response.json().catch(() => ({}));
      throw new APIError(payload.error || `Ошибка сервера: HTTP ${response.status}`, response.status);
    }
    return options.raw ? response : response.json();
  }
  function toast(message, error = false) {
    const element = $('#toast');
    clearTimeout(state.toastTimer);
    element.textContent = message;
    element.className = `visible${error ? ' error' : ''}`;
    state.toastTimer = setTimeout(() => { element.className = ''; }, error ? 7000 : 4000);
  }
  function handleError(error) {
    if (error.name === 'AbortError') return;
    if (error.status === 401 && state.user) {
      state.user = null;
      stopPage();
      closeDialog();
      renderLogin('Сессия завершилась. Войдите снова.');
    } else toast(error.message, true);
  }
  function stopPage() {
    clearTimeout(state.poll);
    clearTimeout(state.searchTimer);
    state.controller?.abort();
    state.stream?.close();
    state.stream = null;
    state.revision++;
    state.loading = false;
    if (state.chartCleanup) state.chartCleanup();
    state.chartCleanup = null;
  }
  function routeFromHash() {
    const hash = location.hash.replace(/^#\/?/, '');
    const [candidate, query] = hash.split('?');
    const route = Object.hasOwn(pages, candidate) ? candidate : 'overview';
    if (route === 'activity' && query) {
      const params = new URLSearchParams(query);
      state.filters.activity = { ...state.filters.activity, user: params.get('user') || '', q: '', offset: 0 };
    }
    return route === 'activity' && !canWrite() ? 'overview' : route;
  }
  function renderShell() {
    const groups = [
      ['МОНИТОРИНГ', ['overview', 'live', 'nodes', 'users', 'agents']],
      ['КОНФИГУРАЦИЯ', ['tiers', 'policies', 'pins', 'geo-rules', 'injections']],
      ['ОПЕРАЦИИ', ['activity', 'ipintel', 'alerts', 'backups', 'audit', 'settings']],
    ];
    app.innerHTML = `<a class="skip-link" href="#main-content">Перейти к содержимому</a><aside class="sidebar" id="sidebar" aria-label="Главное меню"><a class="brand" href="#/overview"><span class="logo-mark">S</span>SubGate<small>CORE</small></a><div class="workspace"><span class="workspace-avatar">${icon('nodes')}</span><span><strong>Operations console</strong><small>Контроль инфраструктуры</small></span></div><nav class="nav">${groups.map(([label, routes]) => `<div class="nav-label">${label}</div>${routes.filter(route => route !== 'activity' || canWrite()).map(route => `<a href="#/${route}" data-route="${route}">${icon(route)}${pages[route][0]}</a>`).join('')}`).join('')}</nav><div class="sidebar-foot"><span class="status-dot" id="connection-dot"></span><span id="connection-label">Core подключён</span><small class="sidebar-version">SubGate · <span id="sidebar-version">—</span></small></div></aside><div class="shell"><div class="topbar"><div class="crumb"><button class="menu-button" type="button" data-action="menu" aria-label="Открыть меню" aria-controls="sidebar" aria-expanded="false">${icon('menu')}</button><span class="root-crumb">Workspace</span><span id="breadcrumb"></span></div><div class="top-right"><span class="clock" id="clock"></span><span class="avatar" title="Владелец">${esc(state.user.Username.slice(0, 2).toUpperCase())}</span><span>${esc(state.user.Username)}</span><button type="button" class="ghost" data-action="logout" aria-label="Выйти из аккаунта" title="Выйти">${icon('logout')}</button></div></div><main class="content" id="main-content" tabindex="-1"></main></div>`;
    updateShell();
  }
  function updateShell() {
    const crumb = $('#breadcrumb');
    if (!crumb) return;
    crumb.textContent = pages[state.route][0];
    document.title = `${pages[state.route][0]} · SubGate`;
    document.querySelectorAll('[data-route]').forEach(link => {
      const active = link.dataset.route === state.route;
      link.classList.toggle('active', active);
      if (active) link.setAttribute('aria-current', 'page'); else link.removeAttribute('aria-current');
    });
    $('#clock').textContent = new Date().toLocaleString('ru-RU', { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' });
    $('#sidebar-version').textContent = state.status?.version || '—';
    $('#connection-label').textContent = state.status?.sync_error ? 'Ошибка синхронизации' : 'Core подключён';
    $('#connection-dot').classList.toggle('warning-dot', !!state.status?.sync_error);
  }
  async function navigate() {
    if (!state.user) return;
    stopPage();
    closeDialog();
    state.route = routeFromHash();
    state.data = {};
    $('#sidebar')?.classList.remove('open');
    $('[data-action="menu"]')?.setAttribute('aria-expanded', 'false');
    updateShell();
    renderPage(`<div class="loading" role="status">Подключение к данным…</div>`);
    await loadPage();
  }
  function schedulePoll() {
    clearTimeout(state.poll);
    state.poll = setTimeout(() => {
      if (!state.user) return;
      if (document.hidden || modal.open || state.loading) { schedulePoll(); return; }
      loadPage(true);
    }, ['overview', 'live'].includes(state.route) ? 5000 : 30000);
  }
  function startLiveStream(revision) {
    if (typeof EventSource === 'undefined' || state.stream || state.route !== 'live') return;
    const stream = new EventSource('/api/v1/live/stream');
    state.stream = stream;
    stream.addEventListener('live', event => {
      if (revision !== state.revision || state.route !== 'live') { stream.close(); return; }
      try {
        const payload = JSON.parse(event.data);
        state.data.live = payload;
        state.updated = new Date();
        renderPage();
      } catch (_) { /* a malformed frame is ignored; the next frame is complete */ }
    });
    stream.onerror = () => {
      if (state.stream === stream) state.stream = null;
      stream.close();
    };
  }
  async function loadPage(background = false) {
    if (!state.user) return;
    state.controller?.abort();
    const controller = new AbortController();
    state.controller = controller;
    const revision = ++state.revision;
    const route = state.route;
    const f = { ...filters() };
    const get = path => request(path, { signal: controller.signal });
    state.loading = true;
    $('[data-action="refresh"]')?.setAttribute('disabled', '');
    try {
      let data;
      const query = new URLSearchParams({ limit: '50', offset: String(f.offset || 0), q: f.q || '' });
      if (route === 'overview') {
        const [status, nodes, agents, live, traffic, alerts, probes] = await Promise.all([get('/system/status'), get('/nodes?limit=1000'), get('/agents'), get('/live'), get('/traffic?minutes=60'), get('/alerts?limit=5'), get('/health/probes?limit=1000')]);
        data = { status, nodes, agents, live, traffic, alerts, probes };
      } else if (route === 'live') {
        const [status, live, traffic] = await Promise.all([get('/system/status'), get('/live'), get(`/traffic?minutes=${f.minutes || 60}`)]);
        data = { status, live, traffic };
      } else if (route === 'agents') {
        const [list, nodes, status] = await Promise.all([get('/agents'), get('/nodes?limit=1000'), get('/system/status')]);
        data = { list, nodes, status };
      } else if (route === 'nodes') {
        const [list, probes] = await Promise.all([get(`/nodes?${query}`), get('/health/probes?limit=1000')]);
        data = { list, probes };
      } else if (route === 'activity') {
        data = { list: await get(`/activity?${activityQuery(f)}`) };
      } else if (route === 'ipintel') {
        data = { ...state.data, status: await get('/system/status') };
      } else if (route === 'settings') {
        const [settings, status] = await Promise.all([get('/settings'), get('/system/status')]);
        data = { settings, status };
      } else if (route === 'backups' || route === 'audit') {
        data = { list: await get(`/${route}?limit=200`) };
      } else {
        data = { list: await get(`/${route}?${query}`) };
      }
      if (revision !== state.revision || route !== state.route) return;
      state.data = data;
      if (data.status) state.status = data.status;
      state.updated = new Date();
      state.loading = false;
      updateShell();
      renderPage();
      if (route === 'live') startLiveStream(revision);
    } catch (error) {
      if (revision !== state.revision || error.name === 'AbortError') return;
      state.loading = false;
      if (error.status === 401) { handleError(error); return; }
      if (background && Object.keys(state.data).length) {
        toast(`Данные не обновились: ${error.message}`, true);
        const foot = $('#updated-at');
        if (foot) foot.textContent = `Последнее успешное обновление ${date(state.updated, true)} · соединение потеряно`;
      } else {
        renderPage(`<div class="notice error" role="alert">${esc(error.message)}</div>${empty('Не удалось загрузить данные', 'Проверьте доступность Core и повторите запрос.', 'error', '<button type="button" data-action="refresh">Повторить</button>')}`);
      }
    } finally {
      if (revision === state.revision && state.user) {
        state.loading = false;
        $('[data-action="refresh"]')?.removeAttribute('disabled');
        schedulePoll();
      }
    }
  }

  function renderPage(override) {
    const main = $('#main-content');
    if (!main) return;
    const focused = document.activeElement;
    const focusID = main.contains(focused) ? focused.id : null;
    const selection = focusID && typeof focused.selectionStart === 'number' ? [focused.selectionStart, focused.selectionEnd] : null;
    if (state.chartCleanup) state.chartCleanup();
    state.chartCleanup = null;
    const functions = { injections: injectionsPage, overview: overviewPage, live: livePage, nodes: nodesPage, users: usersPage, agents: agentsPage, activity: activityPage, ipintel: ipintelPage, backups: backupsPage, audit: auditPage, settings: settingsPage };
    const body = override || (functions[state.route] ? functions[state.route]() : configPage());
    const demo = state.status?.demo ? '<div class="demo-banner">' + icon('error') + '<strong>DEMO</strong><span>Демонстрационные ноды и пользователи. Синтетический трафик не генерируется; измерения появятся после подключения агента.</span></div>' : '';
    main.innerHTML = (override ? pageHead() : '') + demo + body + `<footer class="footer"><span>SubGate · ${esc(state.status?.version || 'Core')} · Владелец</span><span id="updated-at">${state.updated ? `Обновлено ${date(state.updated, true)}` : ''}</span></footer>`;
    if (focusID) {
      const replacement = document.getElementById(focusID);
      if (replacement) {
        replacement.focus({ preventScroll: true });
        if (selection && replacement.setSelectionRange) { try { replacement.setSelectionRange(...selection); } catch (_) { /* datetime and select do not support selection */ } }
      }
    }
    const canvas = $('#traffic-chart');
    if (canvas) drawChart(canvas, state.data.traffic?.data || []);
  }

  function chartPanel() {
    const raw = state.data.traffic?.data || [];
    const grouped = new Map();
    raw.forEach(point => {
      const key = point.timestamp;
      const current = grouped.get(key) || { timestamp: key, uplink_bps: 0, downlink_bps: 0 };
      current.uplink_bps += Math.max(0, finite(point.uplink_bps ?? point.uplink_avg_bps));
      current.downlink_bps += Math.max(0, finite(point.downlink_bps ?? point.downlink_avg_bps));
      grouped.set(key, current);
    });
    const data = [...grouped.values()].sort((a, b) => String(a.timestamp).localeCompare(String(b.timestamp)));
    return panel('Трафик инфраструктуры', `<div class="chart"><canvas id="traffic-chart" role="img" aria-label="История скорости исходящего и входящего трафика в битах в секунду. ${data.length ? `${data.length} измерений за выбранный период.` : 'Измерений пока нет.'}"></canvas>${data.length ? '' : `<div class="chart-empty">${icon('live')}<span>Нет измерений за выбранный период</span><small>Подключите агент и включите Xray StatsService</small></div>`}</div><div class="chart-caption"><span id="chart-reading">${data.length ? `${count(data.length)} точек · шаг 10 секунд` : 'Только реальные данные агентов'}</span><span>бит/с</span></div>`, '<div class="legend"><span>Исходящий</span><span>Входящий</span></div>');
  }
  function overviewPage() {
    const { status, nodes, agents, live, alerts } = state.data;
    if (!status) return '';
    const onlineNodes = (nodes.data || []).filter(node => node.status === 'online').length;
    const g = status.gateway || {};
    const hit = finite(g.requests) ? `${Math.round(finite(g.cache_hits) / finite(g.requests) * 100)}%` : '—';
    const health = `<div class="panel-body"><div class="health-item"><span>Core</span>${badge('online')}</div><div class="health-item"><span>Синхронизация</span>${badge(status.sync_error ? 'error' : status.demo ? 'info' : parseDate(status.last_sync) ? 'success' : 'unknown')}</div><div class="health-item"><span>Последняя синхронизация</span><strong>${esc(date(status.last_sync))}</strong></div><div class="health-item"><span>Uptime Core</span><strong>${esc(duration(status.uptime_seconds))}</strong></div><div class="health-item"><span>Запросы подписок</span><strong>${count(g.requests)}</strong></div><div class="health-item"><span>Попадания в кэш</span><strong>${esc(hit)}</strong></div><div class="health-item"><span>Резервные ответы</span><strong>${count(g.fallbacks)}</strong></div></div>`;
    const alertBody = (alerts.data || []).length ? `<div class="alert-strip">${alerts.data.map(item => `<article><div>${badge(item.severity || 'info')}<strong>${esc(item.name)}</strong></div><p>${esc(item.message || '')}</p></article>`).join('')}</div>` : empty('Оповещений нет', 'В реестре нет текущих событий.', 'alerts');
    return pageHead(canWrite() ? `<button type="button" class="primary" data-action="sync">${icon('refresh')}Синхронизировать</button>` : '') +
      `<div class="metrics">${metric('Ноды в инфраструктуре', count(status.nodes), `${count(onlineNodes)} онлайн по данным панели`, 'nodes')}${metric('Пользователи', count(status.users), 'Синхронизированный реестр Remnawave', 'users')}${metric('Агенты онлайн', count(status.agents_online), `Из ${count((agents.data || []).length)} подключённых коллекторов`, 'agents')}${metric('Потоки телеметрии', count((live.data || []).length), `↓ ${speed(live.downlink_bps)} · ↑ ${speed(live.uplink_bps)}`, 'live')}</div>` +
      (status.sync_error ? `<div class="notice error">Синхронизация с панелью: ${esc(status.sync_error)}</div>` : '') +
      `<div class="grid-main">${chartPanel()}${panel('Состояние системы', health, '<a class="view-link" href="#/settings">Подробнее ' + icon('arrow') + '</a>')}</div>` +
      panel('Флот нод', nodeTable((nodes.data || []).slice(0, 8), false), `<a class="view-link" href="#/nodes">Все ноды ${icon('arrow')}</a>`) +
      `<div class="section-gap">${panel('Оповещения', alertBody, `<a class="view-link" href="#/alerts">Реестр ${icon('arrow')}</a>`)}</div>`;
  }
  function nodeTable(nodes, actions = true) {
    const rows = nodes.map(node => [
      `<div class="row-name"><span class="country-tile">${esc(node.country || '—')}</span><div><strong>${esc(node.name || node.id)}</strong><small class="mono">${esc(node.id)}</small></div></div>`,
      `<span class="mono">${esc(node.ip_address || '—')}</span>`, badge(node.status),
      `<strong>${count(node.users_online)}</strong>`, bytes(node.traffic_used_bytes),
      (node.inbounds || []).map(inbound => `<span class="tag" title="${esc(`${inbound.tag || ''} · ${inbound.network || ''} · ${inbound.security || ''} · :${inbound.port || ''}`)}">${esc(inbound.type || inbound.tag || 'inbound')}</span>`).join('') || '—',
      probeSummary(node.id),
      actions ? resourceActions('nodes', node) : `<a href="#/nodes" class="view-link">Открыть ${icon('arrow')}</a>`,
    ]);
    return table(['Нода', 'Адрес', 'Состояние', 'Онлайн', 'Трафик', 'Протоколы', 'Проверка', 'Действия'], rows, { title: 'Ноды ещё не синхронизированы', detail: 'Укажите адрес Remnawave в конфигурации Core и запустите синхронизацию.', icon: 'nodes' });
  }
  function probeSummary(id) {
    const probe = (state.data.probes?.data || []).find(item => item.node_id === id || item.id === id);
    if (!probe) return '<span class="muted">Не проверено</span>';
    const generic = probe.generic ? '<small>Неизвестный протокол · общая проверка</small>' : '';
    if (probe.verified === 'unverified') return `<span class="badge neutral">Не проверено</span><small>Нужна проба протокола</small>${generic}`;
    return `${badge(probe.success ? 'success' : 'failure')}<small>${esc(String(probe.verified || '').toUpperCase())} · ${count(probe.duration_ms)} мс · ${esc(date(probe.timestamp, true))}</small>${generic}`;
  }
  function resourceActions(kind, item) {
    const common = `data-kind="${kind}" data-id="${esc(item.id)}"`;
    const disabled = String(item.status).toLowerCase() === 'disabled';
    return `<div class="table-actions">${actionButton('details', 'Детали', common)}${canWrite() ? actionButton('resource-action', disabled ? 'Включить' : 'Отключить', `${common} data-operation="${disabled ? 'enable' : 'disable'}"`) + (kind === 'nodes' ? actionButton('probe-node', 'Проверить', common) + actionButton('resource-action', 'Рестарт', `${common} data-operation="restart"`) : '') + actionButton('resource-action', 'Сброс трафика', `${common} data-operation="reset-traffic"`) : ''}${kind === 'users' && canWrite() ? `<a class="row-link" href="#/activity?user=${encodeURIComponent(item.id)}">Журнал</a>` : ''}</div>`;
  }
  function nodesPage() {
    const list = state.data.list || { data: [], total: 0 };
    return pageHead(canWrite() ? '<button type="button" class="primary" data-action="sync">' + icon('refresh') + 'Синхронизировать</button>' : '') + toolbar('Поиск по имени, IP, стране или протоколу') + panel('Ноды Remnawave', nodeTable(list.data || []) + pageFoot(list.total || 0, (list.data || []).length), `<small>${count(list.total)} нод</small>`) + '<p class="help-line">Проверка измеряет установление TCP-соединения и TLS при наличии TLS. Она не подтверждает авторизацию VLESS/REALITY или работу транспортов. Для UDP и QUIC требуется отдельная проверка протокола.</p>';
  }
  function usersPage() {
    const list = state.data.list || { data: [], total: 0 };
    const rows = (list.data || []).map(user => [
      `<strong>${esc(user.username || user.id)}</strong><small>${esc(user.email || '')}</small>`, `<span class="mono">${esc(user.id)}</span>`, badge(user.status),
      `<strong>${esc(bytes(user.used_traffic_bytes))}</strong><small>${finite(user.traffic_limit_bytes) ? `из ${esc(bytes(user.traffic_limit_bytes))} · ${Math.round(finite(user.used_traffic_bytes) / finite(user.traffic_limit_bytes) * 100)}%` : 'Без лимита'}</small>`,
      esc(date(user.expire_at)), `<span class="mono">${esc(user.short_uuid || '—')}</span>`, resourceActions('users', user),
    ]);
    return pageHead() + toolbar('Поиск по имени, email, ID или состоянию') + panel('Пользователи Remnawave', table(['Пользователь', 'ID', 'Состояние', 'Трафик', 'Подписка до', 'Short UUID', 'Действия'], rows, { title: 'Пользователей ещё нет', detail: 'Реестр заполняется при синхронизации с Remnawave.', icon: 'users' }) + pageFoot(list.total || 0, (list.data || []).length), `<small>${count(list.total)} пользователей</small>`);
  }
  function livePage() {
    const live = state.data.live || {};
    const q = String(filters().q || '').toLowerCase();
    const all = live.data || [];
    const data = all.filter(item => !q || `${item.user_id} ${item.user_name || ''} ${item.user_email || ''} ${item.node_id} ${item.node_name || ''} ${item.scope || ''}`.toLowerCase().includes(q));
    const totalUp = all.reduce((sum, item) => sum + finite(item.uplink_bps), 0);
    const totalDown = all.reduce((sum, item) => sum + finite(item.downlink_bps), 0);
    const coarse = all.filter(item => item.coarse || item.scope === 'inbound').length;
    const liveCell = (item, direction) => {
      const current = finite(item[`${direction}_bps`]);
      const ema3 = finite(item[`${direction}_ema_bps`]);
      const ema10 = finite(item[`${direction}_ema10_bps`]);
      const avg = finite(item[`${direction}_avg_bps`]);
      const peak = finite(item[`${direction}_peak_bps`]);
      const p95 = finite(item[`${direction}_p95_bps`]);
      return `<div class="live-cell"><strong class="${direction === 'uplink' ? 'positive' : ''}">${direction === 'uplink' ? '↑' : '↓'} ${esc(speed(current))}</strong><small>EMA 3s ${esc(speed(ema3))} · 10s ${esc(speed(ema10))}</small><small>avg ${esc(speed(avg))} · peak ${esc(speed(peak))} · p95 ${esc(speed(p95))}</small></div>`;
    };
    const rows = data.map(item => {
      const inbound = item.scope === 'inbound' || item.coarse;
      const userLabel = inbound ? `⚠ ${item.user_id || 'inbound'}` : (item.user_name || item.user_email || item.panel_user_id || item.user_id);
      const userDetail = inbound ? 'coarse: inbound tag, не per-user' : (item.user_email || item.panel_user_id || item.user_id);
      const nodeLabel = item.node_name || item.node_id;
      const nodeDetail = [item.node_address, item.node_id].filter(Boolean).join(' · ');
      const history = inbound ? '<span class="muted">не применимо</span>' : actionButton('user-traffic', 'История', `data-user="${esc(item.panel_user_id || item.user_id)}"`);
      return [
        `<strong>${esc(userLabel)}</strong><small class="mono">${esc(userDetail)}</small>`,
        `<strong>${esc(nodeLabel)}</strong><small class="mono">${esc(nodeDetail)}</small>`,
        liveCell(item, 'uplink'), liveCell(item, 'downlink'),
        `<strong>↑ ${esc(bytes(item.uplink_delta_bytes))}</strong><small>↓ ${esc(bytes(item.downlink_delta_bytes))}</small><small>24ч: ↑ ${esc(bytes(item.uplink_total_bytes))} · ↓ ${esc(bytes(item.downlink_total_bytes))}</small>`,
        `${badge(item.status || 'unknown')}<small>${esc(item.age_ms == null ? '—' : `${Math.round(finite(item.age_ms) / 100) / 10} с`)} · ${esc(item.interval_ms || 1000)} ms</small>`,
        `<span class="tag">${esc(item.scope || 'user')}</span>${item.gap ? '<span class="badge amber">gap</span>' : ''}${item.warming ? '<span class="badge neutral">warming</span>' : ''}${item.coarse ? '<span class="badge amber">coarse</span>' : ''}`,
        history,
      ];
    });
    const actions = `<button type="button" data-action="export-live" data-format="csv">${icon('download')}CSV</button><button type="button" data-action="export-live" data-format="json">JSON</button>`;
    return pageHead(actions) + `<div class="metrics">${metric('Исходящий сейчас', speed(totalUp), 'Сумма по всем строкам user/node', 'live')}${metric('Входящий сейчас', speed(totalDown), 'Сумма по всем строкам user/node', 'live')}${metric('Потоки user/node', count(all.length), 'Каждая строка — отдельная нода и scope', 'users')}${metric('Coarse inbound', count(coarse), 'Входящие счётчики без атрибуции пользователя', 'clock')}</div>` +
      `<div class="toolbar"><label>Период графика<select id="traffic-period" data-filter="minutes"><option value="15"${String(filters().minutes) === '15' ? ' selected' : ''}>15 минут</option><option value="60"${!filters().minutes || String(filters().minutes) === '60' ? ' selected' : ''}>1 час</option><option value="360"${String(filters().minutes) === '360' ? ' selected' : ''}>6 часов</option><option value="1440"${String(filters().minutes) === '1440' ? ' selected' : ''}>24 часа</option></select></label><small>Поток обновляется через SSE каждую секунду; строка исчезает после окна свежести.</small></div>` + chartPanel() + `<div class="section-gap">${toolbar('Поиск по пользователю, email, ноде или scope')}${panel('Кто и с какой скоростью передаёт данные сейчас', table(['Пользователь', 'Нода', '↑ Upload', '↓ Download', 'Дельты и totals', 'Состояние', 'Scope', 'История'], rows, { title: 'Свежих измерений нет', detail: 'Проверьте heartbeat агента и Xray StatsService. Первый sample помечается warming; gap не превращается в нулевой трафик.', icon: 'live' }), `<small>${count(data.length)} строк · ${state.stream ? 'SSE подключён' : 'polling'}</small>`)}</div>`;
  }

  function agentsPage() {
    const list = state.data.list?.data || [];
    const nodes = state.data.nodes?.data || [];
    const q = String(filters().q || '').toLowerCase();
    const rows = list.filter(item => !q || JSON.stringify(item).toLowerCase().includes(q)).map(item => {
      const node = nodes.find(node => node.id === item.node_id);
      return [`<strong>${esc(node?.name || item.node_id)}</strong><small class="mono">${esc(item.node_id)}</small>`, badge(item.status === 'revoked' ? 'revoked' : item.online ? item.status : 'offline'), esc(item.version || '—'), esc(date(item.last_heartbeat)), `${count(item.memory_mb)} МиБ`, `<div class="table-actions">${actionButton('agent-diagnostics', 'Диагностика', `data-id="${esc(item.node_id)}"`)}${canWrite() && item.status !== 'revoked' ? actionButton('revoke-agent', 'Отозвать', `data-id="${esc(item.node_id)}"`, 'danger') : ''}</div>`];
    });
    return pageHead(canWrite() ? `<button type="button" class="primary" data-action="enrollment">${icon('plus')}Подключить агент</button>` : '') + `<div class="notice">Агент отправляет скорость из Xray StatsService и события из access.log. Онлайн означает heartbeat за последние 90 секунд. Ошибки источников видны в диагностике.</div>` + toolbar('Поиск по ноде, версии или ошибке') + panel('Подключённые коллекторы', table(['Нода', 'Состояние', 'Версия', 'Heartbeat', 'Память Go', 'Действия'], rows, { title: 'Агенты ещё не подключены', detail: nodes.length ? 'Нажмите «Подключить агент», создайте одноразовый токен и выполните команду на ноде.' : 'Сначала синхронизируйте ноды с Remnawave, затем создайте токен для выбранной ноды.', icon: 'agents' }), `<small>${count(list.filter(item => item.online && item.status !== 'revoked').length)} онлайн</small>`);
  }
  function injectionsPage() {
    const list = state.data.list?.data || [];
    const q = String(filters().q || '').toLowerCase();
    const rows = list.filter(item => `${item.id} ${item.name || ''}`.toLowerCase().includes(q)).map(item => [
      `<strong>${esc(item.name || item.id)}</strong><small>${esc(item.id)}</small>`,
      esc(item.rule_type === 'xray-profile' ? 'Полный Xray-профиль' : `${item.rule_type} · ${item.format || 'авто'}`),
      badge(item.enabled ? 'active' : 'disabled'),
      `<div class="table-actions">${actionButton('edit-config', 'Открыть', `data-kind="injections" data-id="${esc(item.id)}"`)}${canWrite() ? actionButton('toggle-injection', item.enabled ? 'Отключить' : 'Включить', `data-id="${esc(item.id)}"`) + actionButton('delete-config', 'Удалить', `data-kind="injections" data-id="${esc(item.id)}"`, 'danger') : ''}</div>`,
    ]);
    return pageHead(canWrite() ? '<button type="button" class="primary" data-action="new-config" data-kind="injections">Добавить профиль</button>' : '') + '<div class="notice">Добавляется отдельным профилем в подписку Xray JSON. Для URI/base64 и других форматов не применяется. Используйте ссылку /sub/{shortUuid}/v2ray-json. Правило общее; закрепления и политики не нужны.</div>' + toolbar('Поиск по имени или ID') + panel('Инъекции', table(['Название', 'Режим', 'Состояние', 'Действия'], rows));
  }
  async function editProfile(existing) {
    const id = existing?.id || `profile-${crypto.randomUUID()}`;
    const value = existing || { name: '', content: '', enabled: true };
    openDialog('Полный Xray-профиль', `<label>Название правила<input name="name" required maxlength="256" value="${esc(value.name)}" ${canWrite() ? '' : 'readonly'}></label><label class="editor-label">Полный JSON профиля<textarea name="content" class="profile-editor" spellcheck="false" required ${canWrite() ? '' : 'readonly'}>${esc(value.content)}</textarea></label><p class="help-line">Добавляется отдельным профилем в подписку Xray JSON. Для URI/base64 и других форматов не применяется.</p><label class="check-control"><input type="checkbox" name="enabled" ${value.enabled ? 'checked' : ''} ${canWrite() ? '' : 'disabled'}>Включено</label>`, canWrite() ? 'Сохранить' : null, async form => {
      const content = String(form.get('content') || '');
      let parsed;
      try { parsed = JSON.parse(content); } catch (_) { throw new Error('JSON содержит ошибку синтаксиса. Вставьте один полный объект профиля.'); }
      if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object' || !Array.isArray(parsed.outbounds) || !parsed.outbounds.length || parsed.outbounds.some(out => !out || typeof out.protocol !== 'string' || !out.protocol || 'type' in out)) throw new Error('Ожидается один Xray-профиль с непустым outbounds и полем protocol у каждого выхода.');
      if (new TextEncoder().encode(content).length > 256 * 1024) throw new Error('Размер профиля превышает 256 КиБ.');
      const revision = state.dialogRevision;
      // Send the original text: parsing above validates syntax only, preserving large numbers.
      await request(`/injections/${encodeURIComponent(id)}`, { method: 'PUT', body: { name: String(form.get('name')), content, enabled: form.has('enabled'), rule_type: 'xray-profile', format: 'xray-json' } });
      if (revision === state.dialogRevision) closeDialog();
      toast('Профиль сохранён'); await loadPage();
    });
  }
  function configPage() {
    const kind = state.route;
    const list = state.data.list || { data: [], total: 0 };
    const schema = schemas[kind];
    const rows = (list.data || []).map(item => {
      let summary;
      if (kind === 'tiers') summary = `${item.strategy || '—'} · ${(item.node_ids || []).length} нод · min ${item.min_healthy ?? '—'}`;
      else if (kind === 'policies') summary = `Группа: ${item.tier_id || '—'}`;
      else if (kind === 'pins') summary = `${item.user_id || '—'} → ${item.node_id || '—'}`;
      else if (kind === 'geo-rules') summary = `${item.rule_type || '—'} · ${item.action || '—'} · ${(item.values || []).join(', ')}`;
      else summary = item.message || '—';
      return [`<strong>${esc(item.name || item.id)}</strong><small class="mono">${esc(item.id)}</small>`, `<span class="wrap-cell">${esc(summary)}</span>`, kind === 'alerts' ? badge(item.severity || 'info') : badge(item.enabled === true ? 'active' : item.enabled === false ? 'disabled' : 'unknown'), `<div class="table-actions">${actionButton('edit-config', canWrite() ? 'Изменить' : 'Открыть', `data-kind="${kind}" data-id="${esc(item.id)}"`)}${canWrite() ? actionButton('delete-config', 'Удалить', `data-kind="${kind}" data-id="${esc(item.id)}"`, 'danger') : ''}</div>`];
    });
    return pageHead(canWrite() ? `<button type="button" class="primary" data-action="new-config" data-kind="${kind}">${icon('plus')}Создать</button>` : '') + `<details class="notice"><summary>Поддерживаемые поля и поведение</summary><p>${esc(schema.help)}</p></details>` + toolbar('Поиск по имени, ID или содержимому') + panel(pages[kind][0], table(['Запись', 'Конфигурация', kind === 'alerts' ? 'Уровень' : 'Состояние', 'Действия'], rows, { title: 'Записей пока нет', detail: 'Нажмите «Создать» и заполните форму. Расширенный JSON-редактор доступен в окне настройки.', icon: kind }) + pageFoot(list.total || 0, (list.data || []).length), `<small>${count(list.total)} записей</small>`);
  }
  function activityQuery(f = filters(), exportFormat) {
    const query = new URLSearchParams({ limit: exportFormat ? '10000' : '500' });
    for (const key of ['user', 'node', 'domain']) if (f[key]) query.set(key, f[key]);
    for (const key of ['from', 'to']) {
      if (!f[key]) continue;
      const parsed = parseDate(f[key]);
      if (parsed) query.set(key, parsed.toISOString());
    }
    if (exportFormat) query.set('format', exportFormat);
    return query;
  }
  function activityPage() {
    const f = filters();
    const list = state.data.list?.data || [];
    const rows = list.map(item => [esc(date(item.timestamp)), `<strong class="mono">${esc(item.user_id)}</strong><small class="mono">${esc(item.node_id)}</small>`, `<button type="button" class="text-button mono" data-action="lookup-ip" data-ip="${esc(item.src_ip)}">${esc(item.src_ip || '—')}</button>`, `<strong class="mono">${esc(item.dst_host || '—')}</strong><small>${esc(item.network || '')} · :${esc(item.dst_port)}</small>`, esc(item.inbound_tag || '—'), esc(item.outbound_tag || '—'), badge(item.result || 'unknown')]);
    return pageHead(`<button type="button" data-action="export-activity" data-format="csv">${icon('download')}CSV</button><button type="button" data-action="export-activity" data-format="json">${icon('download')}JSON</button>`) +
      `<form id="activity-filters" class="filter-grid"><label>Пользователь (ID)<input id="activity-user" name="user" data-filter="user" value="${esc(f.user)}" placeholder="Все пользователи"></label><label>Нода (ID)<input id="activity-node" name="node" data-filter="node" value="${esc(f.node)}" placeholder="Все ноды"></label><label>Домен / назначение<input id="activity-domain" name="domain" data-filter="domain" value="${esc(f.domain)}" placeholder="example.com"></label><label>С даты (локальное время)<input id="activity-from" name="from" data-filter="from" type="datetime-local" value="${esc(f.from)}"></label><label>По дату (локальное время)<input id="activity-to" name="to" data-filter="to" type="datetime-local" value="${esc(f.to)}"></label><div class="filter-buttons"><button type="submit" class="primary">Применить</button><button type="button" data-action="reset-activity">Сбросить</button>${canWrite() ? '<button type="button" class="danger" data-action="purge-activity">Удалить журнал пользователя</button>' : ''}</div></form>` +
      panel('События соединений', table(['Время', 'Пользователь / нода', 'Источник', 'Назначение', 'Inbound', 'Outbound', 'Результат'], rows, { title: 'Нет событий по выбранным условиям', detail: 'Измените фильтры или проверьте сбор access.log агентом. Для атрибуции журнал Xray должен содержать email пользователя.', icon: 'activity' }), `<small>${count(list.length)} записей · максимум 500</small>`) + `<p class="help-line">Горячий журнал хранится до 14 дней. Экспорт ограничен 10 000 событиями. Домен фильтруется внутри выборки последних событий.</p>`;
  }
  function intelAttribution(sources) {
    return (sources || []).some(source => /db[-_ ]?ip/i.test(typeof source === 'string' ? source : source.database_type || '')) ? '<p class="help-line source-attribution">IP geolocation by <a href="https://db-ip.com/" target="_blank" rel="noopener noreferrer">DB-IP</a></p>' : '';
  }
  function ipintelPage() {
    const result = state.data.result;
    const labels = { ip: 'IP-адрес', asn: 'ASN', as_org: 'Организация', country: 'Страна', region: 'Регион', city: 'Город', network_type: 'Тип сети', confidence: 'Уверенность', timezone: 'Часовой пояс', build_date: 'Сборка источника', sources: 'Источники' };
    const health = state.status?.ipintel || {};
    const sources = Array.isArray(health.sources) ? health.sources : [];
    const sourceRows = sources.map(source => [esc(source.database_type || '—'), esc(date(source.build_date)), source.stale_warning ? `<span class="negative wrap-cell">${esc(source.stale_warning)}</span>` : badge('active')]);
    return pageHead(canWrite() ? `<button type="button" data-action="reload-ipintel">${icon('refresh')}Перезагрузить базы</button>` : '') + `<form id="ip-lookup" class="toolbar"><label class="search-field"><span class="sr-only">IPv4 или IPv6 адрес</span><input id="lookup-input" name="ip" data-filter="ip" value="${esc(filters().ip)}" required placeholder="Введите IPv4 или IPv6" autocomplete="off" spellcheck="false"></label><button type="submit" class="primary">${icon('ipintel')}Проверить</button></form>` + panel('Результат проверки', result ? `<div class="panel-body">${result.stale_warning ? `<div class="notice error">${esc(result.stale_warning)}</div>` : ''}${result.confidence === 'low' ? '<div class="notice">Недостаточно данных для уверенного определения. Неизвестные поля остаются пустыми.</div>' : ''}<div class="key-value">${Object.entries(labels).map(([key, label]) => `<div><small>${label}</small><strong>${esc(key === 'build_date' ? date(result[key]) : Array.isArray(result[key]) ? result[key].join(', ') || 'Нет источников' : result[key] || '—')}</strong></div>`).join('')}</div>${intelAttribution(result.sources)}</div>` : empty('Проверьте IP-адрес', 'Core использует установленные локальные базы. При отсутствии данных результат будет иметь низкую уверенность.', 'ipintel')) + `<div class="section-gap">${panel('Установленные IP-базы', sourceRows.length ? table(['Источник', 'Дата сборки', 'Состояние'], sourceRows) + `<div class="panel-body">${intelAttribution(sources)}</div>` : `<div class="panel-body"><p>MMDB-источники не загружены. Записей префиксов в локальной базе: ${count(health.prefix_records)}.</p></div>`, `<small>${count(health.mmdb_sources ?? sources.length)} MMDB</small>`)}</div>` + '<p class="help-line">Перезагрузка применяет файлы из настроенного каталога IP-данных без перезапуска Core.</p>';
  }
  function backupsPage() {
    const list = state.data.list?.data || [];
    const rows = list.map(item => [`<strong class="mono">${esc(item.name)}</strong>`, esc(date(item.created_at)), bytes(item.size), badge('active').replace('Активен', 'Зашифровано'), canWrite() ? `<button type="button" data-action="download-backup" data-name="${esc(item.name)}">${icon('download')}Скачать</button>` : '<span class="muted">Войдите в аккаунт</span>']);
    return pageHead(canWrite() ? `<button type="button" class="primary" data-action="create-backup">${icon('plus')}Создать копию</button>` : '') + `<div class="notice">Копия содержит согласованный снимок SQLite и шифруется ключом Core. Храните ключ шифрования отдельно: без него восстановление невозможно. </div>` + panel('Снимки базы', table(['Файл', 'Создан', 'Размер', 'Защита', 'Действия'], rows, { title: 'Резервных копий пока нет', detail: 'Создайте первую копию перед важными изменениями конфигурации.', icon: 'backups' }), `<small>${count(list.length)} копий</small>`);
  }
  function auditPage() {
    const q = String(filters().q || '').toLowerCase();
    const list = (state.data.list?.data || []).filter(item => !q || JSON.stringify(item).toLowerCase().includes(q));
    const rows = list.map(item => [esc(date(item.timestamp)), `<span class="mono">#${esc(item.admin_id || '—')}</span>`, `<strong class="mono">${esc(item.action)}</strong>`, `<span class="mono">${esc(item.resource || '—')}</span><small class="mono">${esc(item.resource_id || '')}</small>`, badge(item.status), `<span class="mono">${esc(item.ip || '—')}</span>`]);
    return pageHead() + toolbar('Поиск по действию, ресурсу, IP или администратору') + panel('Последние действия', table(['Время', 'Администратор', 'Действие', 'Ресурс', 'Результат', 'IP'], rows, { title: 'Нет записей по запросу', detail: 'Здесь отображаются последние 200 действий. Попробуйте другой поисковый запрос.', icon: 'audit' }), '<small>Последние 200 записей</small>');
  }
  function settingsPage() {
    const cfg = state.data.settings || {};
    const s = state.data.status || {};
    const groups = [
      ['Core и доступ', [['HTTP listener', cfg.listen], ['Режим', cfg.demo ? 'Demo' : 'Рабочий'], ['Тайм-аут сессии', `${cfg.session_timeout_hours ?? '—'} ч`], ['Учётная запись', 'Владелец — полный доступ'], ['Шифрование активности', cfg.activity_encrypted ? 'Включено' : 'Отключено'], ['Хранение активности', `${cfg.activity_hot_days ?? '—'} дней`]]],
      ['Remnawave и подписки', [['Адрес панели', cfg.panel_url || 'Не настроен'], ['Источник подписок', cfg.subscription_url || 'Не настроен'], ['Интервал синхронизации', `${cfg.sync_interval_seconds ?? '—'} сек`], ['Режим протоколов', cfg.protocol_mode || '—'], ['Последняя синхронизация', date(s.last_sync)], ['Ошибка синхронизации', s.sync_error || 'Нет']]],
      ['Хранение данных', [['Резервные копии', cfg.backup_path || '—'], ['IP Intelligence', cfg.ipintel_data_path || '—'], ['Uptime', duration(s.uptime_seconds)], ['Событий в журнале', count(s.activity_events)]]],
    ];
    return pageHead() + '<div class="notice">Настройки доступны для просмотра. Изменение параметров выполняется в конфигурации сервиса с перезапуском Core. Секреты и пароли в консоль не передаются.</div>' + `<div class="split">${groups.map(([title, pairs]) => panel(title, `<div class="panel-body">${pairs.map(([label, value]) => `<div class="setting-row"><span>${esc(label)}</span><strong>${esc(value)}</strong></div>`).join('')}</div>`)).join('')}${panel('Диагностика компонентов', `<div class="panel-body"><h3>База данных</h3><pre class="code">${esc(JSON.stringify(s.database || {}, null, 2))}</pre><h3>IP Intelligence</h3><pre class="code">${esc(JSON.stringify(s.ipintel || {}, null, 2))}</pre><h3>Шлюз подписок</h3><pre class="code">${esc(JSON.stringify(s.gateway || {}, null, 2))}</pre></div>`)}</div>`;
  }

  function drawChart(canvas, input) {
    const context = canvas.getContext('2d');
    if (!context) return;
    const points = input.map(point => ({ time: parseDate(point.timestamp)?.getTime(), up: Math.max(0, finite(point.uplink_bps ?? point.up)), down: Math.max(0, finite(point.downlink_bps ?? point.down)) })).filter(point => Number.isFinite(point.time)).sort((a, b) => a.time - b.time);
    let dimensions;
    function draw(selected = -1) {
      const rect = canvas.getBoundingClientRect();
      const ratio = Math.min(window.devicePixelRatio || 1, 2);
      const width = Math.max(1, rect.width), height = Math.max(1, rect.height);
      canvas.width = Math.round(width * ratio); canvas.height = Math.round(height * ratio);
      context.setTransform(ratio, 0, 0, ratio, 0, 0);
      context.clearRect(0, 0, width, height);
      const left = Math.min(73, width * 0.23), right = 15, top = 12, bottom = 28;
      const chartW = Math.max(1, width - left - right), chartH = Math.max(1, height - top - bottom);
      const max = Math.max(1, ...points.map(p => Math.max(p.up, p.down))) * 1.12;
      const start = points[0]?.time || Date.now() - 3600000;
      const end = points[points.length - 1]?.time || Date.now();
      const span = Math.max(10000, end - start);
      const x = p => points.length === 1 ? left + chartW / 2 : left + (p.time - start) / span * chartW;
      const y = value => top + chartH * (1 - value / max);
      dimensions = { left, chartW, x };
      context.lineWidth = 1;
      context.font = '10px Segoe UI, sans-serif';
      context.fillStyle = '#718697'; context.textAlign = 'right';
      for (let i = 0; i <= 4; i++) {
        const pos = top + chartH * i / 4;
        context.strokeStyle = '#23303b'; context.beginPath(); context.moveTo(left, pos); context.lineTo(width - right, pos); context.stroke();
        if (points.length) context.fillText(speed(max * (1 - i / 4)).replace('бит/с', '').trim(), left - 10, pos + 3);
      }
      if (!points.length) return;
      context.textAlign = 'left'; context.fillText(date(start, true), left, height - 6);
      context.textAlign = 'right'; context.fillText(date(end, true), width - right, height - 6);
      const paint = (key, color) => {
        const gradient = context.createLinearGradient(0, top, 0, height - bottom);
        gradient.addColorStop(0, `${color}32`); gradient.addColorStop(1, `${color}00`);
        context.beginPath(); context.moveTo(x(points[0]), top + chartH);
        for (const point of points) context.lineTo(x(point), y(point[key]));
        context.lineTo(x(points[points.length - 1]), top + chartH); context.closePath(); context.fillStyle = gradient; context.fill();
        context.beginPath();
        points.forEach((point, index) => { if (!index) context.moveTo(x(point), y(point[key])); else context.lineTo(x(point), y(point[key])); });
        context.strokeStyle = color; context.lineWidth = 2; context.lineJoin = 'round'; context.stroke();
        if (points.length === 1) { context.beginPath(); context.arc(x(points[0]), y(points[0][key]), 3, 0, Math.PI * 2); context.fillStyle = color; context.fill(); }
      };
      paint('down', '#68bddd'); paint('up', '#70e5bd');
      if (selected >= 0 && points[selected]) {
        const point = points[selected];
        context.beginPath(); context.moveTo(x(point), top); context.lineTo(x(point), top + chartH); context.lineWidth = 1; context.strokeStyle = '#7e9b91'; context.setLineDash([3, 3]); context.stroke(); context.setLineDash([]);
        const reading = $('#chart-reading');
        if (reading) reading.textContent = `${date(point.time, true)} · ↑ ${speed(point.up)} · ↓ ${speed(point.down)}`;
      }
    }
    const onMove = event => {
      if (!points.length || !dimensions) return;
      const position = event.clientX - canvas.getBoundingClientRect().left;
      let closest = 0;
      points.forEach((point, index) => { if (Math.abs(dimensions.x(point) - position) < Math.abs(dimensions.x(points[closest]) - position)) closest = index; });
      draw(closest);
    };
    const onLeave = () => { draw(); const reading = $('#chart-reading'); if (reading) reading.textContent = points.length ? `${count(points.length)} точек · шаг 10 секунд` : 'Только реальные данные агентов'; };
    canvas.addEventListener('pointermove', onMove); canvas.addEventListener('pointerleave', onLeave);
    const resize = () => draw();
    const observer = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(resize) : null;
    if (observer) observer.observe(canvas); else window.addEventListener('resize', resize);
    draw();
    state.chartCleanup = () => { observer?.disconnect(); window.removeEventListener('resize', resize); canvas.removeEventListener('pointermove', onMove); canvas.removeEventListener('pointerleave', onLeave); };
  }

  function openDialog(title, content, submit, onSubmit) {
    closeDialog();
    const dialogRevision = state.dialogRevision;
    modalContent.innerHTML = `<h2 id="dialog-title">${esc(title)}</h2>${content}<div class="notice error modal-error" data-modal-error hidden role="alert"></div><div class="dialog-actions"><button type="button" data-action="close-dialog">${submit ? 'Отмена' : 'Закрыть'}</button>${submit ? `<button type="submit" class="primary">${esc(submit)}</button>` : ''}</div>`;
    modal.setAttribute('aria-labelledby', 'dialog-title');
    modalForm.onsubmit = async event => {
      event.preventDefault();
      if (!onSubmit || modalForm.dataset.busy) return;
      const button = $('button[type="submit"]', modalForm);
      const errorBox = $('[data-modal-error]', modalForm);
      modalForm.dataset.busy = '1';
      if (button) button.disabled = true;
      errorBox.hidden = true;
      try { await onSubmit(new FormData(modalForm)); }
      catch (error) { if (error.status === 401) handleError(error); else if (dialogRevision === state.dialogRevision) { errorBox.textContent = error.message; errorBox.hidden = false; } }
      finally { delete modalForm.dataset.busy; if (button) button.disabled = false; }
    };
    modal.showModal();
  }
  function closeDialog() { state.dialogRevision++; state.configEditor = null; if (modal.open) modal.close(); modalForm.onsubmit = null; delete modalForm.dataset.busy; }
  function confirmAction(title, message, callback, submit = 'Подтвердить') {
    openDialog(title, `<p>${esc(message)}</p>`, submit, async () => { const revision = state.dialogRevision; await callback(); if (revision === state.dialogRevision) closeDialog(); });
  }
  function objectByID(kind, id) {
    const list = kind === 'nodes' && state.route === 'overview' ? state.data.nodes?.data : state.data.list?.data;
    return (list || []).find(item => String(item.id) === id);
  }
  function option(value, label, selected) { return `<option value="${esc(value)}"${String(value) === String(selected) ? ' selected' : ''}>${esc(label)}</option>`; }
  function referenceOptions(items, selected, label, exclude = '') {
    const available = items.filter(item => String(item.id) !== String(exclude));
    const missing = selected && !available.some(item => String(item.id) === String(selected));
    return option('', label, selected) + (missing ? option(selected, `${selected} · больше не найден`, selected) : '') + available.map(item => option(item.id, `${item.name || item.username || item.id}${item.ip_address ? ` · ${item.ip_address}` : ''}`, selected)).join('');
  }
  function configFields(kind, value, refs) {
    const text = (name, label, content, extra = '') => `<label>${label}<input id="config-${name}" name="${name}" value="${esc(content ?? '')}" ${extra}></label>`;
    const choice = (name, label, options, required = true) => `<label>${label}<select id="config-${name}" name="${name}"${required ? ' required' : ''}>${options}</select></label>`;
    const enabled = `<label class="check-control"><input type="checkbox" id="config-enabled" name="enabled" ${value.enabled ? 'checked' : ''}><span>Включено</span></label>`;
    const name = text('name', 'Название', value.name, 'required maxlength="200" placeholder="Понятное имя для оператора"');
    if (kind === 'tiers') {
      const checked = new Set(value.node_ids || []);
      const nodes = refs.nodes || [];
      return `<div class="form-grid">${name}${choice('strategy', 'Стратегия выбора', [['leastLoad', 'Меньше пользователей'], ['roundRobin', 'По очереди'], ['random', 'Случайная нода'], ['leastPing', 'Меньше задержка пробы']].map(([key, title]) => option(key, title, value.strategy)).join(''))}${text('min_healthy', 'Минимум онлайн-нод', value.min_healthy ?? 1, 'type="number" min="1" max="1000" step="1" required')}${choice('fallback_tier_id', 'Резервная группа', referenceOptions(refs.tiers || [], value.fallback_tier_id, 'Без резервной группы', value.id), false)}</div><fieldset class="node-picker"><legend>Ноды группы</legend><div class="node-options">${nodes.length ? nodes.map(node => `<label class="check-control node-option"><input type="checkbox" name="node_ids" value="${esc(node.id)}"${checked.has(node.id) ? ' checked' : ''}><span><strong>${esc(node.name || node.id)}</strong><small>${esc(node.ip_address || node.id)} · ${esc(node.status || 'unknown')}</small></span></label>`).join('') : '<p>Ноды отсутствуют. Синхронизируйте Remnawave перед настройкой группы.</p>'}</div></fieldset><p class="help-line">leastPing использует успешную TCP/TLS пробу не старше 10 минут; UDP/QUIC и прикладная авторизация здесь не подтверждаются. При нехватке онлайн-нод проверяется резервная группа.</p>${enabled}`;
    }
    if (kind === 'policies') {
      const conditions = value.conditions || {};
      return `<div class="form-grid">${name}${choice('tier_id', 'Тарифная группа', referenceOptions(refs.tiers || [], value.tier_id, 'Выберите группу'))}${text('priority', 'Приоритет (меньше — раньше)', value.priority ?? 10, 'type="number" min="0" max="1000000" step="1" required')}${text('user_agent', 'User-Agent содержит', conditions.user_agent, 'placeholder="Пусто — любой клиент"')}${text('countries', 'Страны через запятую', (conditions.countries || []).join(', '), 'placeholder="RU, FI, DE"')}${text('asns', 'ASN через запятую', (conditions.asns || []).join(', '), 'placeholder="12345, 67890"')}</div><p class="help-line">Пустые условия подходят всем клиентам. Заполненные условия должны совпасть одновременно. Для страны и ASN нужны локальные IP-данные.</p>${enabled}`;
    }
    if (kind === 'pins') {
      return `<div class="form-grid">${choice('user_id', 'Пользователь', referenceOptions(refs.users || [], value.user_id, 'Выберите пользователя'))}${choice('node_id', 'Нода', referenceOptions(refs.nodes || [], value.node_id, 'Выберите ноду'))}</div><p class="help-line">Закрепление действует, пока нода онлайн, и проверяется раньше тарифных политик.</p>${enabled}`;
    }
    return `<div class="form-grid">${name}${choice('rule_type', 'Тип правила', [['country', 'Страна'], ['asn', 'ASN'], ['cidr', 'IP-сеть (CIDR)'], ['network_type', 'Тип сети']].map(([key, label]) => option(key, label, value.rule_type)).join(''))}${choice('action', 'Действие', option('deny', 'Запретить подписку', value.action) + option('allow', 'Разрешить подписку', value.action))}</div><label>Значения через запятую или с новой строки<textarea id="config-values" name="values" class="values-editor" required placeholder="192.0.2.0/24, 2001:db8::/32">${esc((value.values || []).join(', '))}</textarea></label><p class="help-line">Страна: FI, DE; ASN: 12345; CIDR: 192.0.2.0/24; тип сети: hosting, home_isp, mobile, vpn, cdn, edu, gov. Первое совпадение определяет доступ.</p>${enabled}`;
  }
  function readConfigJSON() {
    let payload;
    try { payload = JSON.parse($('#config-json').value); } catch (_) { throw new Error('JSON содержит ошибку синтаксиса. Проверьте запятые и кавычки.'); }
    if (!payload || Array.isArray(payload) || typeof payload !== 'object') throw new Error('Конфигурация должна быть JSON-объектом.');
    return payload;
  }
  function readConfigFields(validate = true) {
    const editor = state.configEditor;
    if (!canWrite()) return editor.base;
    const form = new FormData(modalForm);
    const get = key => String(form.get(key) ?? '').trim();
    const split = key => [...new Set(get(key).split(/[,;\n]+/).map(value => value.trim()).filter(Boolean))];
    const payload = { ...editor.base, enabled: form.has('enabled') };
    if (editor.kind !== 'pins') payload.name = get('name');
    if (editor.kind === 'tiers') {
      payload.strategy = get('strategy'); payload.min_healthy = Number(get('min_healthy'));
      payload.node_ids = form.getAll('node_ids').map(String); payload.fallback_tier_id = get('fallback_tier_id');
      if (validate && payload.enabled && !payload.node_ids.length) throw new Error('Выберите хотя бы одну ноду для включённой группы.');
    } else if (editor.kind === 'policies') {
      payload.tier_id = get('tier_id'); payload.priority = Number(get('priority'));
      payload.conditions = { countries: split('countries').map(value => value.toUpperCase()), asns: split('asns'), user_agent: get('user_agent') };
      if (validate && payload.conditions.countries.some(value => !/^[A-Z]{2}$/.test(value))) throw new Error('Используйте двухбуквенные коды стран: RU, FI, DE.');
      if (validate && payload.conditions.asns.some(value => !/^[1-9]\d*$/.test(value))) throw new Error('ASN указывается положительным числом без префикса AS.');
    } else if (editor.kind === 'pins') {
      payload.user_id = get('user_id'); payload.node_id = get('node_id');
    } else {
      payload.rule_type = get('rule_type'); payload.action = get('action'); payload.values = split('values');
      if (payload.rule_type === 'country') payload.values = payload.values.map(value => value.toUpperCase());
      if (validate && payload.rule_type === 'country' && payload.values.some(value => !/^[A-Z]{2}$/.test(value))) throw new Error('Используйте двухбуквенные коды стран: RU, FI, DE.');
      if (validate && payload.rule_type === 'asn' && payload.values.some(value => !/^[1-9]\d*$/.test(value))) throw new Error('ASN указывается положительным числом без префикса AS.');
    }
    return payload;
  }
  function setConfigMode(mode) {
    const editor = state.configEditor;
    if (!editor || editor.mode === mode) return;
    const fields = $('#config-fields');
    const json = $('#config-json');
    if (mode === 'json') {
      editor.base = readConfigFields(false);
      json.value = JSON.stringify(editor.base, null, 2);
    } else {
      editor.base = readConfigJSON();
      fields.innerHTML = configFields(editor.kind, editor.base, editor.refs);
    }
    editor.mode = mode;
    fields.hidden = mode !== 'form'; fields.disabled = mode !== 'form' || !canWrite();
    $('#config-json-panel').hidden = mode !== 'json'; json.disabled = mode !== 'json';
    document.querySelectorAll('[data-action="config-mode"]').forEach(button => button.setAttribute('aria-pressed', String(button.dataset.mode === mode)));
  }
  async function configReferences(kind, signal) {
    const needed = { tiers: ['nodes', 'tiers'], policies: ['tiers'], pins: ['nodes', 'users'], 'geo-rules': [] }[kind] || [];
    const results = await Promise.all(needed.map(async source => {
      const data = [];
      let total = 1;
      while (data.length < total) {
        const page = await request(`/${source}?limit=1000&offset=${data.length}`, { signal });
        total = finite(page.total); data.push(...(page.data || []));
        if (!(page.data || []).length) break;
      }
      return [source, data];
    }));
    return Object.fromEntries(results);
  }
  async function editConfig(kind, id) {
    const existing = id ? (kind === 'injections' ? await request(`/injections/${encodeURIComponent(id)}`) : objectByID(kind, id)) : null;
    if (kind === 'injections' && (!existing || existing.rule_type === 'xray-profile')) { await editProfile(existing); return; }
    if (id && !existing) { toast('Запись больше недоступна. Обновите страницу.', true); return; }
    const schema = schemas[kind];
    const value = existing || { ...schema.sample };
    if (kind === 'alerts' && !existing) value.created_at = new Date().toISOString();
    const structured = kind !== 'alerts' && kind !== 'injections';
    let refs = {};
    if (structured) {
      openDialog('Загрузка вариантов', '<div class="loading" role="status">Получаем ноды, пользователей и группы…</div>', null);
      const revision = state.revision; const dialogRevision = state.dialogRevision;
      try { refs = await configReferences(kind, state.controller?.signal); }
      catch (error) { if (dialogRevision === state.dialogRevision) closeDialog(); throw error; }
      if (revision !== state.revision || dialogRevision !== state.dialogRevision) return;
    }
    const shortID = crypto.randomUUID ? crypto.randomUUID().slice(0, 8) : Array.from(crypto.getRandomValues(new Uint8Array(4)), value => value.toString(16).padStart(2, '0')).join('');
    const objectID = id || `${({ tiers: 'tier', policies: 'policy', pins: 'pin', 'geo-rules': 'geo', alerts: 'alert' })[kind]}-${shortID}`;
    const fields = `<label>ID записи<input name="id" id="config-id" pattern="[^/\\\\?#]+" maxlength="128" value="${esc(objectID)}" required ${id || !canWrite() ? 'readonly' : ''}></label>${structured ? `<div class="editor-switch" role="group" aria-label="Режим редактора"><button type="button" data-action="config-mode" data-mode="form" aria-pressed="true">Форма</button><button type="button" data-action="config-mode" data-mode="json" aria-pressed="false">Расширенный JSON</button></div><fieldset id="config-fields" class="config-fields"${canWrite() ? '' : ' disabled'}>${configFields(kind, value, refs)}</fieldset>` : ''}<div id="config-json-panel"${structured ? ' hidden' : ''}><label class="editor-label">Конфигурация JSON<textarea name="json" id="config-json" spellcheck="false"${structured ? ' disabled' : ''} ${canWrite() ? '' : 'readonly'}>${esc(JSON.stringify(value, null, 2))}</textarea></label></div><details class="help-line"><summary>Как применяется конфигурация</summary><p>${esc(schema.help)}</p></details>`;
    openDialog(`${id ? 'Конфигурация' : 'Создать'} · ${pages[kind][0]}`, fields, canWrite() ? 'Сохранить' : null, async form => {
      const dialogRevision = state.dialogRevision;
      const objectID = String(form.get('id') || '').trim();
      if (!objectID || objectID.length > 128 || /[/\\?#]/.test(objectID)) throw new Error('Укажите ID до 128 символов без /, \\, ? и #.');
      const payload = state.configEditor?.mode === 'form' ? readConfigFields() : readConfigJSON();
      if (kind === 'injections') delete payload.id;
      await request(`/${kind}/${encodeURIComponent(objectID)}`, { method: 'PUT', body: payload });
      if (dialogRevision === state.dialogRevision) closeDialog();
      toast('Конфигурация сохранена'); await loadPage();
    });
    if (structured) state.configEditor = { kind, base: value, refs, mode: 'form' };
  }
  function enrollmentDialog() {
    const nodes = state.data.nodes?.data || [];
    if (!nodes.length) { toast('Сначала синхронизируйте ноды с Remnawave.', true); return; }
    openDialog('Подключить агент', `<p>Выберите ноду. Core выдаст одноразовый токен на 15 минут. Повторное подключение этой ноды заменит прежний токен агента.</p><label>Нода<select name="node_id" required>${nodes.map(node => `<option value="${esc(node.id)}">${esc(node.name || node.id)} · ${esc(node.ip_address || node.id)}</option>`).join('')}</select></label>`, 'Создать токен', async form => {
      const revision = state.revision;
      const dialogRevision = state.dialogRevision;
      const result = await request('/agents/enrollment-tokens', { method: 'POST', body: { node_id: String(form.get('node_id')) } });
      if (revision !== state.revision || dialogRevision !== state.dialogRevision) return;
      const quote = value => `'${String(value).replace(/'/g, "'\\''")}'`;
      const installer = 'https://raw.githubusercontent.com/Mrzalupa-lolz/subgate/main/deploy/install-agent.sh';
      const command = `installer=$(mktemp) && curl -fsSL ${quote(installer)} -o "$installer" && sudo bash "$installer" --core-url ${quote(location.origin)} --node-id ${quote(result.node_id)} --enrollment-token ${quote(result.token)}`;
      const update = 'sudo bash /opt/subgate-agent/install-agent.sh --update';
      openDialog('Установка агента на ноду', `<p>Выполните команду на выбранной Linux-ноде с Docker и Compose v2. Она скачает готовый образ, создаст конфигурацию, зарегистрирует агент и запустит его. Токен действует до ${esc(date(result.expires_at))}.</p><pre class="code" id="enrollment-command">${esc(command)}</pre><button type="button" data-action="copy" data-target="enrollment-command">Скопировать установку</button><h3>Обновление установленного агента</h3><pre class="code" id="agent-command">${esc(update)}</pre><button type="button" data-action="copy" data-target="agent-command">Скопировать обновление</button><p class="help-line">Установщик сам определяет стандартный каталог access.log и его группу. Для другого расположения добавьте --log-dir /путь/к/каталогу; для нестандартного StatsService — --stats-addr адрес. Конфигурация хранится в /opt/subgate-agent, токен — в Docker volume. Обновление скачивает свежий latest. Для фиксации версии добавьте --image ghcr.io/mrzalupa-lolz/subgate-agent:ВЕРСИЯ.</p>`, null);
    });
  }
  function diagnosticsDialog(id) {
    const agent = (state.data.list?.data || []).find(item => item.node_id === id);
    if (!agent) { toast('Агент не найден.', true); return; }
    const d = agent.diagnostics || {};
    openDialog('Диагностика агента', `<p class="mono">${esc(id)}</p><div class="setting-row"><span>Состояние</span>${badge(agent.status, agent.online)}</div><div class="setting-row"><span>Heartbeat</span><strong>${esc(date(agent.last_heartbeat))}</strong></div><div class="setting-row"><span>Uptime</span><strong>${esc(duration(d.uptime_seconds))}</strong></div><div class="setting-row"><span>Память Go</span><strong>${count(agent.memory_mb)} МиБ</strong></div><div class="setting-row"><span>Потерянные события</span><strong>${count(d.dropped_events)}</strong></div><h3>StatsService</h3><div class="notice ${d.stats_error ? 'error' : ''}">${esc(d.stats_error || (agent.diagnostics ? 'Ошибок в последнем heartbeat нет.' : 'Heartbeat с диагностикой ещё не получен.'))}</div><h3>Журнал Xray</h3><div class="notice ${d.logs_error ? 'error' : ''}">${esc(d.logs_error || (agent.diagnostics ? 'Ошибок в последнем heartbeat нет.' : 'Нет данных.'))}</div><p>Проверьте локальный адрес StatsService, доступность access.log и наличие пользовательского email в строках лога.</p>`, null);
  }
  async function download(path, filename) {
    const response = await request(path, { raw: true });
    const blob = await response.blob();
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = url; anchor.download = filename; document.body.append(anchor); anchor.click(); anchor.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    toast('Файл подготовлен к скачиванию');
  }
  async function lookupIP(ip) {
    if (!ip) return;
    const revision = state.revision;
    const result = await request(`/ipintel/lookup?${new URLSearchParams({ ip })}`);
    if (revision !== state.revision) return;
    if (state.route === 'ipintel') { state.data.result = result; filters().ip = ip; renderPage(); }
    else openDialog('IP Intelligence', `<p>${esc(ip)}</p>${result.stale_warning ? `<div class="notice error">${esc(result.stale_warning)}</div>` : ''}<pre class="code">${esc(JSON.stringify(result, null, 2))}</pre>${intelAttribution(result.sources)}`, null);
  }
  async function showUserTraffic(user) {
    const revision = state.revision;
    const result = await request(`/users/${encodeURIComponent(user)}/traffic?minutes=1440`);
    if (revision !== state.revision) return;
    const rows = (result.data || []).map(item => [
      esc(date(item.timestamp, true)), `<span class="mono">${esc(item.node_id)}</span>`, `<span class="tag">${esc(item.scope || 'user')}</span>`,
      `<strong class="positive">↑ ${esc(speed(item.uplink_bps))}</strong><small>avg ${esc(speed(item.uplink_avg_bps))} · peak ${esc(speed(item.uplink_peak_bps))} · p95 ${esc(speed(item.uplink_p95_bps))}</small>`,
      `<strong>↓ ${esc(speed(item.downlink_bps))}</strong><small>avg ${esc(speed(item.downlink_avg_bps))} · peak ${esc(speed(item.downlink_peak_bps))} · p95 ${esc(speed(item.downlink_p95_bps))}</small>`,
      `<small>↑ ${esc(bytes(item.uplink_bytes))} · ↓ ${esc(bytes(item.downlink_bytes))}</small><small>${count(item.samples)} samples</small>`,
    ]);
    openDialog(`История трафика · ${user}`, `<p>10-секундные интервалы за последние 24 часа. Строки разделены по ноде и scope; coarse inbound не смешивается с пользовательскими данными.</p>${table(['Время', 'Нода', 'Scope', 'Upload', 'Download', 'Bytes'], rows, { title: 'Истории пока нет', detail: 'Для этого пользователя Core ещё не получил samples.', icon: 'live' })}`, null);
  }
  function validActivityRange() {
    const f = filters();
    if (f.from && f.to && parseDate(f.from)?.getTime() > parseDate(f.to)?.getTime()) { toast('Дата начала должна предшествовать дате окончания.', true); return false; }
    return true;
  }
  async function doAction(button) {
    const { action, kind, id, operation } = button.dataset;
    if (action === 'menu') {
      const open = $('#sidebar').classList.toggle('open');
      button.setAttribute('aria-expanded', String(open)); return;
    }
    if (action === 'close-dialog') { closeDialog(); return; }
    if (action === 'refresh') { await loadPage(); return; }
    if (action === 'export-live') { await download(`/live/export?format=${encodeURIComponent(button.dataset.format || 'json')}`, `subgate-live.${button.dataset.format || 'json'}`); return; }
    if (action === 'user-traffic') { await showUserTraffic(button.dataset.user); return; }
    if (action === 'logout') {
      await request('/auth/logout', { method: 'POST' }); state.user = null; state.status = null; state.data = {}; stopPage(); renderLogin(); return;
    }
    if (action === 'previous' || action === 'next') { filters().offset = Math.max(0, finite(filters().offset) + (action === 'next' ? 50 : -50)); await loadPage(); return; }
    if (action === 'sync') { await request('/panel/sync', { method: 'POST' }); toast('Синхронизация завершена'); await loadPage(); return; }
    if (action === 'details') {
      const revision = state.revision;
      const result = await request(`/${kind}/${encodeURIComponent(id)}`);
      if (revision === state.revision) openDialog(kind === 'nodes' ? 'Данные ноды' : 'Данные пользователя', `<pre class="code">${esc(JSON.stringify(result, null, 2))}</pre>`, null);
      return;
    }
    if (action === 'probe-node') {
      openDialog('Проверка ноды', `<p class="mono">${esc(id)}</p><div class="loading" role="status">Проверяем доступность настроенных endpoint…</div>`, null);
      const revision = state.revision;
      const dialogRevision = state.dialogRevision;
      try {
        const result = await request(`/nodes/${encodeURIComponent(id)}/probe`, { method: 'POST' });
        if (revision !== state.revision || dialogRevision !== state.dialogRevision) return;
        const rows = (result.data || []).map(probe => [esc(`${probe.protocol || '—'} / ${probe.transport || '—'}`) + (probe.generic ? '<small>Неизвестный протокол · общая проверка</small>' : ''), `:${esc(probe.port)}`, probe.verified === 'unverified' ? '<span class="badge neutral">Не проверено</span>' : badge(probe.success ? 'success' : 'failure'), esc(String(probe.verified || '').toUpperCase()), probe.verified === 'unverified' ? '—' : `${count(probe.duration_ms)} мс`, `<span class="wrap-cell">${esc(probe.error || '—')}</span>`]);
        openDialog('Результат проверки ноды', `<p>Подтверждается доступность TCP или TLS endpoint. Авторизация прокси-протокола, REALITY и прикладные транспорты требуют отдельного клиента.</p>${table(['Протокол', 'Порт', 'Результат', 'Проверено', 'Время', 'Ошибка'], rows)}`, null);
        await loadPage();
      } catch (error) { if (dialogRevision === state.dialogRevision) closeDialog(); throw error; }
      return;
    }
    if (action === 'resource-action') {
      const labels = { enable: 'Включить', disable: 'Отключить', restart: 'Перезапустить', 'reset-traffic': 'Сбросить трафик' };
      const apply = async () => { await request(`/${kind}/${encodeURIComponent(id)}/actions/${operation}`, { method: 'POST' }); toast('Операция выполнена'); await loadPage(); };
      if (operation === 'enable') await apply();
      else confirmAction(labels[operation], `${labels[operation]} для ${id}? ${operation === 'reset-traffic' ? 'Счётчик трафика будет обнулён в Remnawave.' : operation === 'restart' ? 'Активные соединения на ноде могут прерваться.' : 'Доступ будет отключён в Remnawave.'}`, apply, labels[operation]);
      return;
    }
    if (action === 'new-config' || action === 'edit-config') { await editConfig(kind, id); return; }
    if (action === 'toggle-injection') {
      const rule = await request(`/injections/${encodeURIComponent(id)}`);
      const { id: ruleID, ...payload } = rule;
      await request(`/injections/${encodeURIComponent(ruleID)}`, { method: 'PUT', body: { ...payload, enabled: !rule.enabled } });
      toast(rule.enabled ? 'Правило отключено' : 'Правило включено'); await loadPage(); return;
    }
    if (action === 'config-mode') { setConfigMode(button.dataset.mode); return; }
    if (action === 'delete-config') { confirmAction('Удалить запись', `Удалить ${id} из реестра ${pages[kind][0]}?`, async () => { await request(`/${kind}/${encodeURIComponent(id)}`, { method: 'DELETE' }); toast('Запись удалена'); await loadPage(); }, 'Удалить'); return; }
    if (action === 'enrollment') { enrollmentDialog(); return; }
    if (action === 'agent-diagnostics') { diagnosticsDialog(id); return; }
    if (action === 'revoke-agent') { confirmAction('Отозвать доступ агента', `Агент ${id} перестанет отправлять heartbeat и телеметрию. Для подключения понадобится новый токен.`, async () => { await request(`/agents/${encodeURIComponent(id)}`, { method: 'DELETE' }); toast('Токен агента отозван'); await loadPage(); }, 'Отозвать'); return; }
    if (action === 'copy') {
      const element = document.getElementById(button.dataset.target);
      if (!element) return;
      const value = element.value ?? element.textContent;
      if (navigator.clipboard?.writeText) { await navigator.clipboard.writeText(value); toast('Скопировано'); }
      else {
        const range = document.createRange(); range.selectNodeContents(element); const selection = window.getSelection(); selection.removeAllRanges(); selection.addRange(range);
        toast('Команда выделена. Нажмите Ctrl+C или выберите «Копировать».');
      }
      return;
    }
    if (action === 'lookup-ip') { await lookupIP(button.dataset.ip); return; }
    if (action === 'reload-ipintel') {
      const result = await request('/ipintel/reload', { method: 'POST' });
      if (state.status) state.status.ipintel = result.health;
      toast('IP-базы перезагружены');
      if (state.route === 'ipintel' && filters().ip) await lookupIP(filters().ip); else await loadPage();
      return;
    }
    if (action === 'reset-activity') { state.filters.activity = { q: '', offset: 0 }; await loadPage(); return; }
    if (action === 'export-activity') { if (validActivityRange()) await download(`/activity/export?${activityQuery(filters(), button.dataset.format)}`, `subgate-activity.${button.dataset.format}`); return; }
    if (action === 'purge-activity') {
      const user = String(filters().user || '').trim();
      if (!user) { toast('Укажите ID пользователя в фильтре перед удалением.', true); $('#activity-user')?.focus(); return; }
      confirmAction('Удалить журнал пользователя', `Все сохранённые события пользователя ${user} будут удалены, включая записи вне выбранного диапазона. Это действие невозможно отменить.`, async () => { await request(`/activity/${encodeURIComponent(user)}`, { method: 'DELETE' }); toast('Журнал пользователя удалён'); await loadPage(); }, 'Удалить события'); return;
    }
    if (action === 'create-backup') { const result = await request('/backups', { method: 'POST' }); toast(`Копия создана: ${result.name}`); await loadPage(); return; }
    if (action === 'download-backup') { await download(`/backups/${encodeURIComponent(button.dataset.name)}`, button.dataset.name); }
  }

  document.addEventListener('click', async event => {
    if (event.target.closest('.skip-link')) { event.preventDefault(); $('#main-content')?.focus(); return; }
    const button = event.target.closest('button[data-action]');
    if (!button || button.disabled) return;
    button.disabled = true;
    try { await doAction(button); } catch (error) { handleError(error); }
    finally { if (button.isConnected) button.disabled = false; }
  });
  document.addEventListener('input', event => {
    const field = event.target.closest('[data-filter]');
    if (!field || !state.user) return;
    filters()[field.dataset.filter] = field.value;
    if (field.dataset.filter !== 'q') return;
    filters().offset = 0;
    clearTimeout(state.searchTimer);
    if (['live', 'agents', 'audit'].includes(state.route)) { renderPage(); return; }
    state.searchTimer = setTimeout(() => loadPage(), 300);
  });
  document.addEventListener('change', event => {
    if (event.target.dataset.filter === 'minutes') { filters().minutes = event.target.value; loadPage(); }
  });
  document.addEventListener('keydown', event => {
    if (event.key === 'Escape' && !modal.open) { $('#sidebar')?.classList.remove('open'); $('[data-action="menu"]')?.setAttribute('aria-expanded', 'false'); }
  });
  document.addEventListener('submit', async event => {
    const form = event.target;
    if (!['activity-filters', 'ip-lookup'].includes(form.id)) return;
    event.preventDefault();
    try {
      if (form.id === 'activity-filters') { if (validActivityRange()) await loadPage(); }
      else {
        const button = $('button[type="submit"]', form); button.disabled = true;
        try { await lookupIP(String(new FormData(form).get('ip') || '').trim()); }
        finally { if (button.isConnected) button.disabled = false; }
      }
    } catch (error) { handleError(error); }
  });
  modal.addEventListener('click', event => { if (event.target === modal) { const rect = modal.getBoundingClientRect(); if (event.clientX < rect.left || event.clientX > rect.right || event.clientY < rect.top || event.clientY > rect.bottom) closeDialog(); } });
  modal.addEventListener('cancel', () => { closeDialog(); });
  window.addEventListener('hashchange', navigate);
  document.addEventListener('visibilitychange', () => { if (!document.hidden && state.user && !state.loading && !modal.open) loadPage(true); });

  function renderLogin(message = '') {
    document.title = 'Вход · SubGate';
    app.innerHTML = `<main class="login"><section class="login-story"><div class="brand"><span class="logo-mark">S</span>SubGate</div><div class="eyebrow">Infrastructure operations</div><h1>Ваша инфраструктура.<br><span>Полная картина.</span></h1><p>Шлюз подписок, флот нод и телеметрия Xray в одной консоли управления.</p><small>Remnawave · Xray · SubGate Core</small></section><section class="login-form-area"><form id="login-form"><h2>Войти в консоль</h2><p>Используйте аккаунт администратора SubGate.</p>${message ? `<div class="notice error" role="alert">${esc(message)}</div>` : ''}<label>Имя пользователя<input name="username" id="login-username" autocomplete="username" maxlength="128" required autofocus></label><label>Пароль<input name="password" type="password" autocomplete="current-password" maxlength="1024" required></label><div id="login-error" class="notice error" role="alert" hidden></div><button type="submit" class="primary">Войти ${icon('arrow')}</button><div class="demo-hint" id="demo-login-hint" hidden>Demo: admin / subgate-demo</div></form></section></main>`;
    $('#login-form').addEventListener('submit', async event => {
      event.preventDefault();
      const form = event.currentTarget;
      const button = $('button[type="submit"]', form);
      const errorBox = $('#login-error');
      errorBox.hidden = true; button.disabled = true; button.textContent = 'Вход…';
      try {
        const fields = new FormData(form);
        state.user = principal(await request('/auth/login', { method: 'POST', body: { username: String(fields.get('username')).trim(), password: String(fields.get('password')) } }));
        state.status = await request('/system/status');
        renderShell(); await navigate();
      } catch (error) {
        if (form.isConnected) { errorBox.textContent = error.status === 401 ? 'Неверное имя пользователя или пароль.' : error.message; errorBox.hidden = false; }
      } finally { if (button.isConnected) { button.disabled = false; button.innerHTML = `Войти ${icon('arrow')}`; } }
    });
    fetch('/version', { credentials: 'same-origin', cache: 'no-store' }).then(response => response.ok ? response.json() : {}).then(version => { const hint = $('#demo-login-hint'); if (hint) hint.hidden = !version.demo; }).catch(() => {});
  }
  async function start() {
    try {
      state.user = principal(await request('/auth/me'));
      state.status = await request('/system/status');
      renderShell(); await navigate();
    } catch (error) {
      state.user = null;
      renderLogin(error.status === 401 ? '' : error.message);
    }
  }
  start();
})();
