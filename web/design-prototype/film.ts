import { mount } from 'svelte';
import Film from './Film.svelte';
import './styles.css';

mount(Film, { target: document.getElementById('film')! });
