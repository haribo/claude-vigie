module.exports = {
  extends: ['@commitlint/config-conventional'],
  rules: {
    // 80, not 72: squash-merges append " (#NNN)" to the subject, which would
    // otherwise push an ~72-char header over the limit on the develop→main PR.
    'header-max-length': [2, 'always', 80],
    'type-enum': [
      2,
      'always',
      [
        'feat',
        'fix',
        'docs',
        'style',
        'refactor',
        'perf',
        'test',
        'chore',
        'ci',
        'build',
      ],
    ],
    'body-max-line-length': [0],
    'footer-max-line-length': [0],
  },
};
