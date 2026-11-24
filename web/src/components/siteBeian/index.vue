<!--
  网站备案信息展示（ICP 备案号 / 公网安备号）
  独立成组件的原因：bottomInfo.vue 头部有版权声明，不修改该文件；
  通过其 <slot /> 注入，登录页与后台主布局共用同一份备案信息；
  JXC 业务页使用独立外壳（view/jxc/shell.vue），在其 footer 中单独引用。
  备案号变更只需改此文件。
-->
<template>
  <div class="site-beian text-center">
    <a
      class="text-slate-500 hover:text-active dark:text-slate-500"
      href="https://beian.miit.gov.cn/"
      target="_blank"
      rel="noopener noreferrer"
      >粤ICP备2026181944号-1</a
    >
    <span class="mx-2 text-slate-400">|</span>
    <a
      class="text-slate-500 hover:text-active dark:text-slate-500"
      href="https://beian.mps.gov.cn/"
      target="_blank"
      rel="noopener noreferrer"
      >粤公网安备44011102485502号</a
    >
  </div>
</template>

<script setup>
  defineOptions({
    name: 'SiteBeian'
  })
</script>
