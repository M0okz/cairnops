export { cn } from '../../src/lib/utils';
export type WithElementRef<T, E extends HTMLElement = HTMLElement> = T & { ref?: E | null };
export type WithoutChild<T> = Omit<T, 'child'>;
export type WithoutChildren<T> = Omit<T, 'children'>;
export type WithoutChildrenOrChild<T> = Omit<T, 'children' | 'child'>;
