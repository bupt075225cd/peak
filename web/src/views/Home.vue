<script setup lang="ts">
import { useAuth } from '../composables/useAuth'
import GuestLanding from '../components/home/GuestLanding.vue'
import Dashboard from '../components/home/Dashboard.vue'

const { isLoggedIn } = useAuth()
</script>

<template>
  <GuestLanding v-if="!isLoggedIn" />
  <Dashboard v-else />
</template>
