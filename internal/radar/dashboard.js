// All views and exports use data embedded in this offline report.
const byID = id => document.getElementById(id);
const download = byID('calendar-download');
const share = byID('calendar-share');
const downloadStatus = byID('download-status');
const calendarText = byID('calendar-text');

if (!byID('calendar-export').hidden) {
  calendarText.addEventListener('focus', () => calendarText.select());
  download.addEventListener('click', () => {
    downloadStatus.textContent = 'If the download is unavailable, use the calendar text below.';
  });

  try {
    const blob = new Blob([calendarText.value.replace(/\r?\n/g, '\r\n')], {
      type: 'text/calendar;charset=utf-8'
    });
    download.href = URL.createObjectURL(blob);
    if (typeof navigator.canShare === 'function' && typeof navigator.share === 'function') {
      const file = new File([blob], 'music-concerts.ics', { type: 'text/calendar' });
      if (navigator.canShare({ files: [file] })) {
        share.hidden = false;
        share.addEventListener('click', async () => {
          try {
            await navigator.share({ files: [file], title: 'Music concerts' });
            downloadStatus.textContent = 'Calendar sent to your device’s share sheet.';
          } catch (error) {
            if (error.name !== 'AbortError') {
              downloadStatus.textContent = 'Sharing is unavailable here. Download the file or use the text below.';
            }
          }
        });
      }
    }
  } catch (error) {
    // The embedded download and the text fallback also work without Blob URLs.
  }
}

const search = byID('search');
const period = byID('period');
const source = byID('source');
const country = byID('country');
const clear = byID('reset-filters');
const agenda = byID('agenda');
const calendar = byID('calendar');
const grid = byID('month-grid');
const listButton = byID('list-view');
const calendarButton = byID('calendar-view');
const snapshotDate = calendar.dataset.today;
const snapshotMonth = snapshotDate.slice(0, 7);
const monthFormat = new Intl.DateTimeFormat('en', { month: 'long', year: 'numeric', timeZone: 'UTC' });
const dayFormat = new Intl.DateTimeFormat('en', {
  weekday: 'long', month: 'long', day: 'numeric', year: 'numeric', timeZone: 'UTC'
});

function normalize(text) {
  return text.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase().trim();
}

const groups = Array.from(agenda.querySelectorAll('.agenda-group'), group => ({
  group,
  shows: Array.from(group.querySelectorAll('.concert'), node => {
    const { date, endDate, timing, status, sources, country } = node.dataset;
    return {
      node, date, endDate: endDate || date, timing, status, country, sources: sources.split('|'),
      searchText: normalize(Array.from(node.querySelectorAll('.artist, h3, .venue'), part => part.textContent).join(' '))
    };
  })
}));

let shown = groups.flatMap(({ shows }) => shows);
let activeMonth = snapshotMonth;
let selectedDate = '';
let view = 'agenda';

const dateObject = iso => new Date(iso + 'T00:00:00Z');
const dateKey = date => date.toISOString().slice(0, 10);
const dayKey = (month, day) => month + '-' + String(day).padStart(2, '0');

function filtered() {
  return !!(search.value.trim() || period.value || source.value || country.value);
}

function monthBounds(month) {
  const start = dayKey(month, 1);
  const first = dateObject(start);
  const days = new Date(Date.UTC(first.getUTCFullYear(), first.getUTCMonth() + 1, 0)).getUTCDate();
  return { start, end: dayKey(month, days), first, days };
}

function followMatches(dated) {
  const { start, end } = monthBounds(activeMonth);
  if (!dated.length || dated.some(show => show.date <= end && show.endDate >= start)) return;

  // Prefer the next matching show, falling back to the latest past date.
  const next = dated.map(show => show.date).filter(date => date > end).sort()[0];
  const previous = dated.map(show => show.endDate).sort().at(-1);
  activeMonth = (next || previous).slice(0, 7);
  selectedDate = '';
}

function renderCalendar({ follow = false, focus = false } = {}) {
  const dated = shown.filter(show => show.date.length === 10);
  if (follow) followMatches(dated);
  const { start, end, first, days } = monthBounds(activeMonth);
  const byDate = new Map();
  for (const show of dated) {
    if (show.endDate < start || show.date > end) continue;
    const firstDay = Number((show.date > start ? show.date : start).slice(8));
    const lastDay = Number((show.endDate < end ? show.endDate : end).slice(8));
    for (let day = firstDay; day <= lastDay; day++) {
      const key = dayKey(activeMonth, day);
      if (!byDate.has(key)) byDate.set(key, []);
      byDate.get(key).push(show);
    }
  }
  const matches = [...byDate.keys()].sort();
  if (!selectedDate.startsWith(activeMonth + '-') || (follow && !byDate.has(selectedDate))) {
    selectedDate = matches.find(date => date >= snapshotDate) || matches[0] ||
      (snapshotMonth === activeMonth ? snapshotDate : start);
  }

  byID('month-label').textContent = monthFormat.format(first);
  const cells = document.createDocumentFragment();
  for (const name of ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']) {
    const weekday = document.createElement('span');
    weekday.className = 'weekday';
    weekday.textContent = name;
    cells.append(weekday);
  }
  const offset = (first.getUTCDay() + 6) % 7;
  for (let i = 0; i < offset; i++) {
    const blank = document.createElement('span');
    blank.setAttribute('aria-hidden', 'true');
    cells.append(blank);
  }
  for (let day = 1; day <= days; day++) {
    const date = dayKey(activeMonth, day);
    const count = byDate.get(date)?.length || 0;
    const button = document.createElement('button');
    button.className = count ? 'day-select has-concerts' : 'day-select';
    button.dataset.date = date;
    button.tabIndex = date === selectedDate ? 0 : -1;
    button.setAttribute('aria-label', dayFormat.format(dateObject(date)) + ', ' + concertCount(count));
    button.setAttribute('aria-pressed', String(date === selectedDate));
    button.setAttribute('aria-controls', 'day-details');
    if (date === snapshotDate) button.setAttribute('aria-current', 'date');
    const number = document.createElement('span');
    number.className = 'day-number';
    number.textContent = day;
    button.append(number);
    if (count) {
      const badge = document.createElement('span');
      badge.className = 'day-count';
      badge.textContent = count;
      badge.setAttribute('aria-hidden', 'true');
      button.append(badge);
    }
    cells.append(button);
  }
  grid.replaceChildren(cells);
  if (focus) grid.querySelector('[aria-pressed="true"]')?.focus();

  const selected = byDate.get(selectedDate) || [];
  byID('selected-day-label').textContent = dayFormat.format(dateObject(selectedDate));
  byID('day-empty').hidden = selected.length > 0;
  byID('calendar-empty').hidden = matches.length > 0;
  byID('day-concert-list').replaceChildren(...selected.map(show => {
    const article = document.createElement('article');
    article.className = 'day-concert';
    article.append(show.node.querySelector('.concert-body').cloneNode(true));
    return article;
  }));
  const uncertain = shown.filter(show => show.date.length < 10 && (
    !show.date || show.date === activeMonth || show.date === activeMonth.slice(0, 4)
  ));
  byID('uncertain-dates').hidden = uncertain.length === 0;
  byID('uncertain-list').replaceChildren(...uncertain.map(show => {
    const article = show.node.cloneNode(true);
    article.className = 'concert uncertain-item';
    return article;
  }));
}

function moveMonth(delta) {
  const date = dateObject(dayKey(activeMonth, 1));
  date.setUTCMonth(date.getUTCMonth() + delta);
  activeMonth = dateKey(date).slice(0, 7);
  selectedDate = '';
  renderCalendar();
}

function concertCount(count) {
  return count + (count === 1 ? ' concert' : ' concerts');
}

function filter() {
  const terms = normalize(search.value).split(/\s+/).filter(Boolean);
  const [criterion, value] = period.value.split(':');
  shown = [];
  for (const { group, shows } of groups) {
    const matches = shows.filter(show => {
      const timingMatch = criterion === 'timing' && value === 'Upcoming' &&
        (show.timing === 'Today' || show.timing === 'Ongoing');
      const match = (!criterion || show[criterion] === value || timingMatch) &&
        (!source.value || show.sources.includes(source.value)) &&
        (!country.value || show.country === country.value) &&
        terms.every(term => show.searchText.includes(term));
      show.node.hidden = !match;
      return match;
    });
    group.hidden = matches.length === 0;
    shown.push(...matches);
  }

  const filtering = filtered();
  byID('count').textContent = concertCount(shown.length);
  clear.hidden = !filtering;
  byID('empty').hidden = shown.length > 0;
  byID('empty').textContent = filtering ? 'No concerts match these filters.' :
    'No concerts in this report. Check lookup status below for progress.';
  if (view === 'calendar') renderCalendar({ follow: filtering });
}

function switchView(next) {
  view = next;
  agenda.hidden = view !== 'agenda';
  calendar.hidden = !agenda.hidden;
  listButton.setAttribute('aria-pressed', String(!agenda.hidden));
  calendarButton.setAttribute('aria-pressed', String(!calendar.hidden));
  if (view === 'calendar') renderCalendar({ follow: filtered() });
}

grid.addEventListener('click', event => {
  const button = event.target.closest('.day-select');
  if (!button) return;
  selectedDate = button.dataset.date;
  renderCalendar({ focus: true });
});

grid.addEventListener('keydown', event => {
  const button = event.target.closest('.day-select');
  const steps = { ArrowLeft: -1, ArrowRight: 1, ArrowUp: -7, ArrowDown: 7 };
  if (!button || !Object.hasOwn(steps, event.key)) return;
  event.preventDefault();
  const next = dateObject(button.dataset.date);
  next.setUTCDate(next.getUTCDate() + steps[event.key]);
  selectedDate = dateKey(next);
  activeMonth = selectedDate.slice(0, 7);
  renderCalendar({ focus: true });
});

search.addEventListener('input', filter);
period.addEventListener('change', filter);
source.addEventListener('change', filter);
country.addEventListener('change', filter);
clear.addEventListener('click', () => {
  search.value = period.value = source.value = country.value = '';
  filter();
  search.focus();
});
listButton.addEventListener('click', () => switchView('agenda'));
calendarButton.addEventListener('click', () => switchView('calendar'));
byID('previous-month').addEventListener('click', () => moveMonth(-1));
byID('next-month').addEventListener('click', () => moveMonth(1));
byID('current-month').addEventListener('click', () => {
  activeMonth = snapshotMonth;
  selectedDate = '';
  renderCalendar();
});
if (typeof Intl.DisplayNames === 'function') {
  const regions = new Intl.DisplayNames(['en'], { type: 'region' });
  const options = Array.from(country.options).slice(1);
  for (const option of options) option.textContent = regions.of(option.value);
  options.sort((a, b) => a.textContent.localeCompare(b.textContent, 'en'));
  country.append(...options);
}
byID('calendar-month').append(byID('calendar-export'));
byID('dashboard-controls').hidden = false;
