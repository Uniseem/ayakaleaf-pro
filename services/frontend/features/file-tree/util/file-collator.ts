/**
 * How names are ordered in the tree.
 *
 * English as the base language whatever the reader's locale is, so that a
 * project's files are in the same order for everybody looking at it. Numeric
 * so that chapter10 comes after chapter2 rather than after chapter1, and
 * case-sensitive with upper case first, so two names differing only in case
 * do not sort as one.
 */
export const fileCollator = new Intl.Collator('en', {
  numeric: true,
  sensitivity: 'variant',
  caseFirst: 'upper',
})
