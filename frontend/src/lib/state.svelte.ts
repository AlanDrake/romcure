import * as Service from '../../bindings/romcure/cmd/romcure/service';
import type {State} from '../../bindings/romcure/cmd/romcure/models';

export type Tab = 'library' | 'games' | 'outputs' | 'settings' | 'help';

/** A link in a notice: opens a tab and, with anchor, scrolls to the element with that id. */
export type NoticeLink = {label: string; tab: Tab; anchor?: string};

/** Opens the tab a notice link points to and scrolls to its anchor once the tab has rendered. */
export function followLink(l: NoticeLink) {
  app.tab = l.tab;
  if (!l.anchor) return;
  const id = l.anchor;
  // The view mounts on the next tick(s); retry briefly until the element exists.
  let tries = 0;
  const go = () => {
    const el = document.getElementById(id);
    if (el) el.scrollIntoView({block: 'start', behavior: 'smooth'});
    else if (tries++ < 20) setTimeout(go, 50);
  };
  setTimeout(go, 0);
}

// Shared UI state: what the shell and every view read and write.
export const app = $state({
  tab: 'library' as Tab,
  st: null as State | null,
  error: '',
  notice: '',
  // Background job ("sync" | "dats" | "chd"), "" when idle
  job: '',
  progress: '',
  // The latest progress event of the running job (see JobProgress in service.go).
  step: {Stage: '', SystemID: '', Profile: '', Detail: '', Item: '', Done: 0, Total: 0, Percent: 0},
  jobStarted: 0,
  noticeLink: null as NoticeLink | null,
  noticeWarn: false,
  // True while the first-run setup is open (also reopened from Help).
  setupOpen: false,
  datResults: [] as any[],
  chdResults: [] as any[],
  // Bumped when a sync finishes, so open views can reload their data.
  syncDone: 0,
  // Set by other tabs to open the Games screen on an output, a system or a filter.
  gamesFocus: null as {outputId?: number; system?: string; unverified?: boolean} | null,
});

export const jobLabel: Record<string, string> = {sync: 'Syncing', apply: 'Updating outputs', dats: 'Updating DATs', chd: 'Checking CHD files'};

export async function refresh() {
  try {
    app.st = await Service.State();
    app.job = app.st.Job;
  } catch (e) {
    fail(e);
  }
}

/** The text of an error thrown by a service call (or anything else thrown). */
export const message = (e: unknown) => (e instanceof Error ? e.message : String(e));

export function fail(e: unknown) {
  app.error = message(e);
  app.notice = '';
  app.noticeLink = null;
}

/** Shows a message; link opens the tab with the details, warn paints it as a caution. */
export function ok(msg: string, link: NoticeLink | null = null, warn = false) {
  app.notice = msg;
  app.noticeLink = link;
  app.noticeWarn = warn;
  app.error = '';
}

export function resetStep() {
  app.step = {Stage: '', SystemID: '', Profile: '', Detail: '', Item: '', Done: 0, Total: 0, Percent: 0};
}

export async function start(kind: string, run: () => Promise<void>) {
  try {
    app.error = '';
    app.notice = '';
    app.noticeLink = null;
    resetStep();
    app.jobStarted = Date.now();
    app.job = kind;
    await run();
  } catch (e) {
    app.job = '';
    fail(e);
  }
}

/** Restarts the app as administrator (Windows); remembers a declined prompt. */
export async function relaunchElevated() {
  try {
    const r = await Service.RelaunchElevated();
    if (r === 'declined') {
      ok('No problem: outputs will be copied instead of linked.');
      await refresh();
    }
  } catch (e) {
    fail(e);
  }
}

/** Remembers "never ask": outputs use copies instead of links. */
export async function keepCopies() {
  try {
    await Service.KeepCopies();
    ok('Outputs will be copied instead of linked. You can change this in Settings.');
    await refresh();
  } catch (e) {
    fail(e);
  }
}
