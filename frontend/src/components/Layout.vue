<template>
  <div>
    <a href="https://github.com/achernya/nonstick" target="_blank">
      <img src="../assets/nonstick.svg" class="logo" alt="Nonstick logo" />
    </a>
    <slot></slot>
  </div>
</template>

<style scoped>
.logo {
  height: 6em;
  padding: 1.5em;
  will-change: filter;
  transition: filter 300ms;
}
.logo:hover {
  filter: drop-shadow(0 0 2em #646cffaa);
}
</style>
