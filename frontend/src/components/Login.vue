<script setup>
import { ref } from 'vue'

const connect = ref(0)
const items = ref([])

var websocket;

function wsUrl(path = "/api/pamws") {
    var params = new URL(document.location).searchParams;
    const challenge = params.get("login_challenge")
    const protocol = (window.location.protocol === 'https:') ? 'wss:' : 'ws:';
    return protocol + '//' + location.host + path + "?login_challenge=" + challenge;
}

function onConnect() {
    connect.value = true
    websocket = new WebSocket(wsUrl());
    websocket.onopen = (event) => {
	console.log("Connected")
    };
    websocket.onmessage = (event) => {
	console.log(event.data)
	items.value.push(JSON.parse(event.data));
    };
}

function toWebsocket(e) {
    e.preventDefault();
    websocket.send(JSON.stringify({"Input": e.currentTarget.elements[0].value}));
    for (let i = 0; i < e.currentTarget.elements.length; i++) {
	e.currentTarget.elements[i].disabled = true;
    }
}

function onReset() {
    connect.value = false;
    websocket.close();
    websocket = null;
    items.value = [];
}

</script>

<template>
  <h1>Nonstick IdP</h1>

  <Transition>
    <div v-if="!connect">
      <div class="modal-background">
	<div class="modal-content">
	  <button @click="onConnect">Log in</button>
	</div>
      </div>
    </div>
  </Transition>
  <Transition>
    <div v-if="connect">
      <p v-for="item in items">
	<pre class="pam-form">{{ item.Message }} </pre>
	<form class="pam-form" v-if="item.Type.startsWith('PromptEcho')" v-on:submit="toWebsocket">
	  <input name="input" :type="[item.Type.endsWith('Off') ? 'password' : 'text']">
	  <button type="submit">Submit</button>
	</form>
	<form v-if="item.Type == 'Error'" v-on:submit="onReset">
	  <button type="submit">Reset</button>
	</form>
      </p>
    </div>
  </Transition>
 </template>

<style scoped>
.v-enter-active,
.v-leave-active {
  transition: opacity 0.5s ease;
}

.v-enter-from,
.v-leave-to {
  opacity: 0;
}

.pam-form {
  display: inline;
}
</style>
