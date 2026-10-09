module.exports = {
  testEnvironment: 'node',
  collectCoverageFrom: [
    'bookmarks/src/**/*.js',
    'collections/src/**/*.js',
    '!**/node_modules/**',
  ],
  testMatch: ['**/__tests__/**/*.test.js'],
};
