export function getStartOfLocalDayUTC(date: Date = new Date()): Date {
  const localMidnight = new Date(
    date.getFullYear(),
    date.getMonth(),
    date.getDate(),
    0,
    0,
    0,
    0,
  );
  return new Date(
    localMidnight.getTime() - localMidnight.getTimezoneOffset() * 60000,
  );
}
